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

	// Generate rejected response (skip for SFT mode if model not configured)
	var rejectedDuration time.Duration
	rejectedModel, hasRejectedModel := o.cfg.Models["rejected"]

	if hasRejectedModel {
		rejectedStart := time.Now()
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
		rejectedDuration = time.Since(rejectedStart)
	} else {
		// SFT mode without rejected model
		result.Rejected = ""
		rejectedDuration = 0
	}

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
			// Apply optional judge filtering (all modes except MO-DPO)
			shouldFilter := false
			if o.cfg.JudgeFiltering.Enabled && o.cfg.Generation.DatasetMode != models.DatasetModeMODPO {
				shouldFilter = o.applyJudgeFiltering(result.Job.Prompt, result.Chosen, result.Rejected)
				if shouldFilter {
					o.stats.FilteredCount++
					o.logger.Debug("Filtered record",
						"job_id", result.Job.ID,
						"reason", "below score thresholds")
				}
			}

			if !shouldFilter {
				// Write based on dataset mode
				err := o.writeRecordByMode(result)
				if err != nil {
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
		}

		_ = bar.Add(1)
	}
}

// applyJudgeFiltering evaluates and filters based on score thresholds
func (o *Orchestrator) applyJudgeFiltering(prompt, chosen, rejected string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Evaluate chosen response
	chosenScore, err := o.judgeModule.EvaluateForFiltering(ctx, prompt, chosen)
	if err != nil {
		o.logger.Warn("Judge filtering failed for chosen response", "error", err)
		return false // Don't filter on error
	}

	// Evaluate rejected response (if present)
	var rejectedScore float64
	if rejected != "" {
		rejectedScore, err = o.judgeModule.EvaluateForFiltering(ctx, prompt, rejected)
		if err != nil {
			o.logger.Warn("Judge filtering failed for rejected response", "error", err)
			return false // Don't filter on error
		}
	}

	// Filter if chosen score too low OR rejected score too high
	shouldFilter := chosenScore < o.cfg.JudgeFiltering.MinChosenScore ||
		(rejected != "" && rejectedScore > o.cfg.JudgeFiltering.MaxRejectedScore)

	return shouldFilter
}

// writeRecordByMode writes the record based on the configured dataset mode
func (o *Orchestrator) writeRecordByMode(result models.GenerationResult) error {
	switch o.cfg.Generation.DatasetMode {
	case models.DatasetModeSFT:
		return o.writeSFTRecord(result)
	case models.DatasetModeDPO:
		return o.writeDPORecord(result)
	case models.DatasetModeKTO:
		return o.writeKTORecord(result)
	case models.DatasetModeMODPO:
		return o.writeMODPORecord(result)
	default:
		return fmt.Errorf("unknown dataset mode: %s", o.cfg.Generation.DatasetMode)
	}
}

// writeSFTRecord writes a simple instruction-output record
func (o *Orchestrator) writeSFTRecord(result models.GenerationResult) error {
	record := models.SFTRecord{
		Instruction: result.Job.Prompt,
		Output:      result.Chosen,
	}

	// Optionally include topic columns
	if o.cfg.Generation.IncludeTopicColumns {
		record.MainTopic = result.Job.MainTopic
		record.SubTopic = result.Job.SubTopic
	}

	return o.dataWriter.WriteSFTRecord(record)
}

// writeDPORecord writes a standard DPO preference pair
func (o *Orchestrator) writeDPORecord(result models.GenerationResult) error {
	record := models.DPORecord{
		Prompt:   result.Job.Prompt,
		Chosen:   result.Chosen,
		Rejected: result.Rejected,
	}
	return o.dataWriter.WriteDPORecord(record)
}

// writeKTORecord writes two KTO records (one chosen, one rejected)
func (o *Orchestrator) writeKTORecord(result models.GenerationResult) error {
	// Write chosen record
	chosenRecord := models.KTORecord{
		Prompt:     result.Job.Prompt,
		Completion: result.Chosen,
		Label:      true,
	}
	if err := o.dataWriter.WriteKTORecord(chosenRecord); err != nil {
		return fmt.Errorf("failed to write KTO chosen record: %w", err)
	}

	// Write rejected record
	rejectedRecord := models.KTORecord{
		Prompt:     result.Job.Prompt,
		Completion: result.Rejected,
		Label:      false,
	}
	if err := o.dataWriter.WriteKTORecord(rejectedRecord); err != nil {
		return fmt.Errorf("failed to write KTO rejected record: %w", err)
	}

	return nil
}

// writeMODPORecord writes a full MO-DPO record with judge evaluation
func (o *Orchestrator) writeMODPORecord(result models.GenerationResult) error {
	// Write initial record (without judge results)
	record := models.DatasetRecord{
		MainTopic: result.Job.MainTopic,
		SubTopic:  result.Job.SubTopic,
		Prompt:    result.Job.Prompt,
		Chosen:    result.Chosen,
		Rejected:  result.Rejected,
	}

	// Note: Judge results will be added asynchronously via background goroutines
	// WriteRecord returns the record index for later updates
	recordIndex, err := o.dataWriter.WriteRecord(record)
	if err != nil {
		return err
	}

	// Spawn background judge goroutine (non-blocking!)
	if o.judgeModule != nil {
		o.pendingJudges.Add(1)
		go o.evaluateJudgeAsync(recordIndex, result.Job.Prompt, result.Chosen, result.Rejected)
	}

	return nil
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
