package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lamim/vellumforge2/internal/api"
	"github.com/lamim/vellumforge2/internal/util"
	"github.com/lamim/vellumforge2/pkg/models"
	"github.com/schollz/progressbar/v3"
)

var (
	// ErrJudgeTimeout indicates that judge evaluation exceeded timeout
	ErrJudgeTimeout = errors.New("judge evaluation timeout")
)

func (o *Orchestrator) worker(
	ctx context.Context,
	workerID int,
	jobs <-chan models.GenerationJob,
	results chan<- models.GenerationResult,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	workerLogger := o.logger.With("worker_id", workerID)
	workerLogger.Debug("Worker started")

	for job := range jobs {
		select {
		case <-ctx.Done():
			workerLogger.Info("Worker cancelled")
			return
		default:
		}

		// Determine max retries for judge timeout
		maxRetries := 0
		if o.judgeModule != nil {
			judgeModel := o.cfg.Models["judge"]
			maxRetries = judgeModel.MaxRetries
			if maxRetries == 0 {
				maxRetries = 3 // fallback default
			}
		}

		// Retry loop for judge timeouts
		var result models.GenerationResult
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				workerLogger.Warn("Retrying job due to judge timeout",
					"job_id", job.ID,
					"attempt", attempt,
					"max_retries", maxRetries)
			}

			startTime := time.Now()
			result = o.processJob(ctx, workerLogger, job, attempt)
			result.Duration = time.Since(startTime)

			// Success or non-timeout error - don't retry
			if result.Error == nil || !errors.Is(result.Error, ErrJudgeTimeout) {
				break
			}

			// Judge timeout - retry if attempts remain
			if attempt < maxRetries {
				continue
			}

			// All retries exhausted
			workerLogger.Error("Job failed after all retries",
				"job_id", job.ID,
				"attempts", attempt+1,
				"error", "judge consistently timed out")
		}

		results <- result
	}

	workerLogger.Debug("Worker finished")
}

func (o *Orchestrator) processJob(
	ctx context.Context,
	logger *slog.Logger,
	job models.GenerationJob,
	attempt int,
) models.GenerationResult {
	jobStartTime := time.Now()
	result := models.GenerationResult{
		Job: job,
	}

	// Generate chosen response (main model)
	chosenStart := time.Now()
	mainModel := o.cfg.Models["main"]
	mainAPIKey := o.secrets.GetAPIKey(mainModel.BaseURL)

	// Render chosen generation prompt
	chosenPrompt, err := util.RenderTemplate(o.cfg.PromptTemplates.ChosenGeneration, map[string]interface{}{
		"Prompt": job.Prompt,
	})
	if err != nil {
		result.Error = fmt.Errorf("failed to render chosen template: %w", err)
		return result
	}

	chosenResp, err := o.apiClient.ChatCompletion(ctx, mainModel, mainAPIKey, []api.Message{
		{Role: "user", Content: chosenPrompt},
	})
	if err != nil {
		result.Error = fmt.Errorf("failed to generate chosen response: %w", err)
		return result
	}
	result.Chosen = chosenResp.Choices[0].Message.Content
	chosenDuration := time.Since(chosenStart)

	// Generate rejected response (rejected model)
	rejectedStart := time.Now()
	rejectedModel := o.cfg.Models["rejected"]
	rejectedAPIKey := o.secrets.GetAPIKey(rejectedModel.BaseURL)

	// Render rejected generation prompt
	rejectedPrompt, err := util.RenderTemplate(o.cfg.PromptTemplates.RejectedGeneration, map[string]interface{}{
		"Prompt": job.Prompt,
	})
	if err != nil {
		result.Error = fmt.Errorf("failed to render rejected template: %w", err)
		return result
	}

	rejectedResp, err := o.apiClient.ChatCompletion(ctx, rejectedModel, rejectedAPIKey, []api.Message{
		{Role: "user", Content: rejectedPrompt},
	})
	if err != nil {
		result.Error = fmt.Errorf("failed to generate rejected response: %w", err)
		return result
	}
	result.Rejected = rejectedResp.Choices[0].Message.Content
	rejectedDuration := time.Since(rejectedStart)

	// Run judge evaluation with timeout and cancellation
	// When judge is enabled, we MUST get results for dataset consistency
	var judgeDuration time.Duration
	if o.judgeModule != nil {
		judgeStart := time.Now()

		// Create cancellable context for judge API call
		judgeCtx, cancelJudge := context.WithCancel(ctx)
		defer cancelJudge() // Ensure cleanup

		judgeDone := make(chan *models.JudgeResult, 1)

		// Launch judge evaluation with cancellable context
		go func() {
			judgeResult, err := o.judgeModule.Evaluate(judgeCtx, job.Prompt, result.Chosen, result.Rejected)
			if err != nil {
				// Don't log if cancelled (expected on timeout)
				if judgeCtx.Err() != context.Canceled {
					logger.Warn("Judge evaluation failed",
						"job_id", job.ID,
						"error", err)
				}
				judgeDone <- nil
			} else {
				judgeDone <- judgeResult
			}
		}()

		// Wait for judge with 100ms timeout
		select {
		case judgeResult := <-judgeDone:
			judgeDuration = time.Since(judgeStart)
			if judgeResult == nil {
				// Judge returned error - fail the job
				result.Error = fmt.Errorf("judge evaluation failed")
				return result
			}
			result.JudgeResult = judgeResult
			logger.Debug("Judge completed",
				"job_id", job.ID,
				"duration_ms", judgeDuration.Milliseconds())
		case <-time.After(100 * time.Millisecond):
			// Timeout - cancel HTTP request and return timeout error for retry
			cancelJudge()
			judgeDuration = time.Since(judgeStart)
			logger.Debug("Judge timeout - will retry job",
				"job_id", job.ID,
				"attempt", attempt,
				"waited_ms", judgeDuration.Milliseconds())
			result.Error = fmt.Errorf("%w: exceeded 100ms", ErrJudgeTimeout)
			return result
		}
	}

	totalDuration := time.Since(jobStartTime)

	// Log detailed timing breakdown for benchmark analysis
	logger.Info("Job processing breakdown",
		"job_id", job.ID,
		"chosen_ms", chosenDuration.Milliseconds(),
		"rejected_ms", rejectedDuration.Milliseconds(),
		"judge_ms", judgeDuration.Milliseconds(),
		"total_ms", totalDuration.Milliseconds())

	return result
}

func (o *Orchestrator) collectResults(results <-chan models.GenerationResult, wg *sync.WaitGroup) {
	defer wg.Done()

	bar := progressbar.Default(int64(o.stats.TotalPrompts), "Processing")

	for result := range results {
		if result.Error != nil {
			o.logger.Error("Job failed",
				"job_id", result.Job.ID,
				"error", result.Error)
			o.stats.FailureCount++
		} else {
			// Write to dataset
			record := models.DatasetRecord{
				MainTopic: result.Job.MainTopic,
				SubTopic:  result.Job.SubTopic,
				Prompt:    result.Job.Prompt,
				Chosen:    result.Chosen,
				Rejected:  result.Rejected,
			}

			// Add judge results if available
			if result.JudgeResult != nil {
				record.ChosenScores = result.JudgeResult.ChosenScores
				record.RejectedScores = result.JudgeResult.RejectedScores
				record.ChosenScoreTotal = result.JudgeResult.ChosenScoreTotal
				record.RejectedScoreTotal = result.JudgeResult.RejectedScoreTotal
				record.PreferenceMargin = result.JudgeResult.PreferenceMargin
			}

			if err := o.dataWriter.WriteRecord(record); err != nil {
				o.logger.Error("Failed to write record",
					"job_id", result.Job.ID,
					"error", err)
				o.stats.FailureCount++
			} else {
				o.stats.SuccessCount++

				// Checkpoint progress (interval-based)
				if o.checkpointMgr != nil {
					if err := o.checkpointMgr.MarkJobComplete(result.Job.ID, o.stats); err != nil {
						o.logger.Warn("Failed to checkpoint job", "job_id", result.Job.ID, "error", err)
					}
				}
			}
		}

		_ = bar.Add(1)
	}
}
