package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lamim/vellumforge2/internal/api"
	"github.com/lamim/vellumforge2/internal/util"
	"github.com/lamim/vellumforge2/pkg/models"
	"github.com/schollz/progressbar/v3"
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

		startTime := time.Now()
		result := o.processJob(ctx, workerLogger, job)
		result.Duration = time.Since(startTime)

		results <- result
	}

	workerLogger.Debug("Worker finished")
}

func (o *Orchestrator) processJob(
	ctx context.Context,
	logger *slog.Logger,
	job models.GenerationJob,
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

	// Optional: Run judge evaluation asynchronously
	var judgeDuration time.Duration
	if o.judgeModule != nil {
		judgeStart := time.Now()
		
		// Create a channel to receive judge result
		judgeDone := make(chan *models.JudgeResult, 1)
		
		// Launch judge evaluation in background
		go func() {
			judgeResult, err := o.judgeModule.Evaluate(ctx, job.Prompt, result.Chosen, result.Rejected)
			if err != nil {
				logger.Warn("Judge evaluation failed",
					"job_id", job.ID,
					"error", err)
				judgeDone <- nil
			} else {
				judgeDone <- judgeResult
			}
		}()
		
		// Wait for judge result with timeout or return immediately
		select {
		case judgeResult := <-judgeDone:
			judgeDuration = time.Since(judgeStart)
			result.JudgeResult = judgeResult
		case <-time.After(100 * time.Millisecond):
			// Don't block worker - judge will complete in background
			// Mark as incomplete so we know it's async
			logger.Debug("Judge evaluation running async",
				"job_id", job.ID)
			judgeDuration = time.Since(judgeStart)
			// Continue without judge result
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
