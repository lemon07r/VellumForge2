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

		// Process job (judge runs async, no retries needed)
		startTime := time.Now()
		result := o.processJob(ctx, workerLogger, job, 0)
		result.Duration = time.Since(startTime)

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

	totalDuration := time.Since(jobStartTime)

	// Log detailed timing breakdown for benchmark analysis (judge runs async, not included)
	logger.Info("Job processing breakdown",
		"job_id", job.ID,
		"chosen_ms", chosenDuration.Milliseconds(),
		"rejected_ms", rejectedDuration.Milliseconds(),
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
			// Write to dataset (without judge results initially)
			record := models.DatasetRecord{
				MainTopic: result.Job.MainTopic,
				SubTopic:  result.Job.SubTopic,
				Prompt:    result.Job.Prompt,
				Chosen:    result.Chosen,
				Rejected:  result.Rejected,
			}

			// Note: Judge results will be added asynchronously via background goroutines
			// WriteRecord now returns the record index for later updates
			recordIndex, err := o.dataWriter.WriteRecord(record)
			if err != nil {
				o.logger.Error("Failed to write record",
					"job_id", result.Job.ID,
					"error", err)
				o.stats.FailureCount++
			} else {
				o.stats.SuccessCount++

				// Spawn background judge goroutine (non-blocking!)
				if o.judgeModule != nil {
					o.pendingJudges.Add(1)
					go o.evaluateJudgeAsync(recordIndex, result.Job.Prompt, result.Chosen, result.Rejected)
				}

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

// judgeUpdater runs in a background goroutine and processes judge result updates
func (o *Orchestrator) judgeUpdater(ctx context.Context) {
	for {
		select {
		case update, ok := <-o.judgeUpdates:
			if !ok {
				// Channel closed, updater shutting down
				return
			}
			
			// Update the record with judge results
			err := o.dataWriter.UpdateRecord(update.recordIndex, update.judgeResult)
			if err != nil {
				o.logger.Error("Failed to update record with judge results",
					"record_index", update.recordIndex,
					"error", err)
			} else {
				o.logger.Debug("Updated record with judge results",
					"record_index", update.recordIndex)
			}
		case <-ctx.Done():
			return
		}
	}
}

// evaluateJudgeAsync evaluates judge in background (non-blocking for workers)
// This function spawns as a goroutine and runs independently
func (o *Orchestrator) evaluateJudgeAsync(recordIndex int, prompt, chosen, rejected string) {
	defer o.pendingJudges.Done()

	// Acquire semaphore slot to limit concurrent judge goroutines
	o.judgeSemaphore <- struct{}{}
	defer func() { <-o.judgeSemaphore }()

	// Create context with timeout for judge evaluation
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Evaluate (this blocks for 70-103s, but doesn't block workers!)
	judgeResult, err := o.judgeModule.Evaluate(ctx, prompt, chosen, rejected)
	if err != nil {
		o.logger.Warn("Background judge evaluation failed",
			"record_index", recordIndex,
			"error", err)
		return
	}

	// Send update to updater goroutine
	select {
	case o.judgeUpdates <- judgeUpdate{
		recordIndex: recordIndex,
		judgeResult: judgeResult,
	}:
		// Update queued successfully
	case <-ctx.Done():
		o.logger.Warn("Judge update dropped due to context cancellation",
			"record_index", recordIndex)
	}
}
