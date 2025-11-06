package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/lamim/vellumforge2/internal/api"
	"github.com/lamim/vellumforge2/internal/checkpoint"
	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/internal/judge"
	"github.com/lamim/vellumforge2/internal/util"
	"github.com/lamim/vellumforge2/internal/writer"
	"github.com/lamim/vellumforge2/pkg/models"
	"github.com/schollz/progressbar/v3"
)

const (
	// judgeUpdateBufferSize is the channel buffer size for async judge result updates.
	// This provides backpressure to prevent unbounded memory growth while allowing
	// judge operations to complete asynchronously without blocking workers.
	judgeUpdateBufferSize = 100
)

// judgeUpdate represents an async judge result update
type judgeUpdate struct {
	recordIndex int
	judgeResult *models.JudgeResult
}

// Orchestrator manages the entire data generation pipeline
type Orchestrator struct {
	cfg           *config.Config
	secrets       *config.Secrets
	apiClient     *api.Client
	judgeModule   *judge.Judge
	dataWriter    *writer.DatasetWriter
	logger        *slog.Logger
	stats         *models.SessionStats
	checkpointMgr *checkpoint.Manager
	resumeMode    bool
	ctx           context.Context // Main context for cancellation propagation
	// Non-blocking judge support
	judgeUpdates   chan judgeUpdate
	pendingJudges  sync.WaitGroup
	judgeSemaphore chan struct{} // Limit concurrent judge goroutines
}

// New creates a new orchestrator
func New(
	cfg *config.Config,
	secrets *config.Secrets,
	apiClient *api.Client,
	dataWriter *writer.DatasetWriter,
	checkpointMgr *checkpoint.Manager,
	resumeMode bool,
	logger *slog.Logger,
) *Orchestrator {
	var judgeModule *judge.Judge
	if judgeModel, ok := cfg.Models["judge"]; ok && judgeModel.Enabled {
		judgeModule = judge.New(cfg, secrets, apiClient, logger)
	}

	stats := &models.SessionStats{
		StartTime: time.Now(),
	}

	// In resume mode, restore stats from checkpoint
	if resumeMode && checkpointMgr != nil {
		cp := checkpointMgr.GetCheckpoint()
		stats = &cp.Stats
		// Keep original start time but update for this run
		stats.StartTime = time.Now()
	}

	o := &Orchestrator{
		cfg:           cfg,
		secrets:       secrets,
		apiClient:     apiClient,
		judgeModule:   judgeModule,
		dataWriter:    dataWriter,
		logger:        logger,
		stats:         stats,
		checkpointMgr: checkpointMgr,
		resumeMode:    resumeMode,
	}

	// Initialize non-blocking judge support if judge is enabled
	if judgeModule != nil {
		o.judgeUpdates = make(chan judgeUpdate, judgeUpdateBufferSize)
		// Judge semaphore scales with main concurrency to prevent bottleneck
		o.judgeSemaphore = make(chan struct{}, cfg.Generation.Concurrency)
	}

	return o
}

// Run executes the complete generation pipeline
func (o *Orchestrator) Run(ctx context.Context) error {
	// Store context for judge goroutines to respect cancellation
	o.ctx = ctx

	var checkpointCloseErr error

	defer func() {
		// Ensure checkpoint manager is properly closed
		if o.checkpointMgr != nil {
			// First, try to save final checkpoint synchronously
			if err := o.checkpointMgr.SaveSync(); err != nil {
				o.logger.Error("Failed to save final checkpoint", "error", err)
				checkpointCloseErr = err
			}

			// Then close the manager
			if err := o.checkpointMgr.Close(); err != nil {
				o.logger.Error("Failed to close checkpoint manager", "error", err)
				if checkpointCloseErr == nil {
					checkpointCloseErr = err
				}
			}
		}
	}()

	o.logger.Info("Starting generation pipeline",
		"main_topic", o.cfg.Generation.MainTopic,
		"num_subtopics", o.cfg.Generation.NumSubtopics,
		"prompts_per_subtopic", o.cfg.Generation.NumPromptsPerSubtopic,
		"resume_mode", o.resumeMode)

	// Phase 1: Generate subtopics
	var subtopics []string
	var err error

	if o.resumeMode && o.checkpointMgr != nil {
		cp := o.checkpointMgr.GetCheckpoint()
		if cp.SubtopicsComplete {
			subtopics = cp.Subtopics
			o.logger.Info("Resuming from checkpoint: subtopics phase complete", "count", len(subtopics))
		} else {
			subtopics, err = o.generateSubtopics(ctx)
			if err != nil {
				return fmt.Errorf("failed to generate subtopics: %w", err)
			}
			if o.checkpointMgr != nil {
				if err := o.checkpointMgr.MarkSubtopicsComplete(subtopics); err != nil {
					o.logger.Warn("Failed to save subtopics checkpoint", "error", err)
				}
			}
		}
	} else {
		subtopics, err = o.generateSubtopics(ctx)
		if err != nil {
			return fmt.Errorf("failed to generate subtopics: %w", err)
		}
		if o.checkpointMgr != nil {
			if err := o.checkpointMgr.MarkSubtopicsComplete(subtopics); err != nil {
				o.logger.Warn("Failed to save subtopics checkpoint", "error", err)
			}
		}
	}

	o.logger.Info("Generated subtopics", "count", len(subtopics))

	// Validate subtopic count
	if len(subtopics) != o.cfg.Generation.NumSubtopics {
		o.logger.Warn("Subtopic count mismatch",
			"expected", o.cfg.Generation.NumSubtopics,
			"actual", len(subtopics),
			"difference", len(subtopics)-o.cfg.Generation.NumSubtopics)
	}

	// Phase 2: Generate prompts for each subtopic
	var prompts []models.GenerationJob

	if o.resumeMode && o.checkpointMgr != nil {
		cp := o.checkpointMgr.GetCheckpoint()
		if cp.PromptsComplete {
			prompts = cp.Prompts
			o.logger.Info("Resuming from checkpoint: prompts phase complete", "count", len(prompts))
		} else {
			prompts, err = o.generatePrompts(ctx, subtopics)
			if err != nil {
				return fmt.Errorf("failed to generate prompts: %w", err)
			}
			if o.checkpointMgr != nil {
				if err := o.checkpointMgr.MarkPromptsComplete(prompts); err != nil {
					o.logger.Warn("Failed to save prompts checkpoint", "error", err)
				}
			}
		}
	} else {
		prompts, err = o.generatePrompts(ctx, subtopics)
		if err != nil {
			return fmt.Errorf("failed to generate prompts: %w", err)
		}
		if o.checkpointMgr != nil {
			if err := o.checkpointMgr.MarkPromptsComplete(prompts); err != nil {
				o.logger.Warn("Failed to save prompts checkpoint", "error", err)
			}
		}
	}

	o.logger.Info("Generated prompts", "count", len(prompts))
	o.stats.TotalPrompts = len(prompts)

	// Validate prompt count
	expectedPrompts := len(subtopics) * o.cfg.Generation.NumPromptsPerSubtopic
	if len(prompts) != expectedPrompts {
		o.logger.Warn("Prompt count mismatch",
			"expected", expectedPrompts,
			"actual", len(prompts),
			"difference", len(prompts)-expectedPrompts)
	}

	// Phase 3: Generate preference pairs concurrently (with resume filtering)
	pendingJobs := prompts
	if o.resumeMode && o.checkpointMgr != nil {
		cp := o.checkpointMgr.GetCheckpoint()
		pendingJobs = checkpoint.GetPendingJobs(cp)
		completed := len(prompts) - len(pendingJobs)
		o.logger.Info("Resuming from checkpoint: preference pairs phase",
			"total", len(prompts),
			"completed", completed,
			"pending", len(pendingJobs),
			"progress", fmt.Sprintf("%.1f%%", checkpoint.GetProgressPercentage(cp)))
	}

	// Start judge updater goroutine if judge is enabled
	updaterCtx, cancelUpdater := context.WithCancel(ctx)
	defer cancelUpdater()
	if o.judgeModule != nil {
		go o.judgeUpdater(updaterCtx)
	}

	if err := o.generatePreferencePairs(ctx, pendingJobs); err != nil {
		return fmt.Errorf("failed to generate preference pairs: %w", err)
	}

	// Wait for all pending background judges to complete
	if o.judgeModule != nil {
		o.logger.Info("Waiting for background judge evaluations to complete...")
		o.pendingJudges.Wait()
		close(o.judgeUpdates)
		o.logger.Info("All background judge evaluations complete")
	}

	// Finalize stats
	o.stats.EndTime = time.Now()
	o.stats.TotalDuration = o.stats.EndTime.Sub(o.stats.StartTime)
	if o.stats.SuccessCount > 0 {
		o.stats.AverageDuration = o.stats.TotalDuration / time.Duration(o.stats.SuccessCount)
	}

	// Mark checkpoint as complete
	if o.checkpointMgr != nil {
		if err := o.checkpointMgr.MarkComplete(o.stats); err != nil {
			o.logger.Warn("Failed to save final checkpoint", "error", err)
		}
	}

	o.logger.Info("Generation pipeline completed",
		"total_prompts", o.stats.TotalPrompts,
		"successful", o.stats.SuccessCount,
		"failed", o.stats.FailureCount,
		"duration", o.stats.TotalDuration,
		"average_per_prompt", o.stats.AverageDuration)

	// Final validation summary
	if o.stats.FailureCount > 0 {
		failureRate := float64(o.stats.FailureCount) / float64(o.stats.TotalPrompts) * 100
		o.logger.Warn("Generation completed with failures",
			"failure_rate", fmt.Sprintf("%.2f%%", failureRate),
			"lost_rows", o.stats.FailureCount)
	}

	// Check for checkpoint save/close errors before returning
	if checkpointCloseErr != nil {
		return fmt.Errorf("checkpoint save failed during shutdown: %w", checkpointCloseErr)
	}

	return nil
}

func (o *Orchestrator) generateSubtopics(ctx context.Context) ([]string, error) {
	targetCount := o.cfg.Generation.NumSubtopics

	// STRATEGY: Request extra based on configurable buffer to account for LLM undershoot and duplicates
	multiplier := 1.0 + o.cfg.Generation.OverGenerationBuffer
	requestCount := int(float64(targetCount) * multiplier)

	bufferPercent := int(o.cfg.Generation.OverGenerationBuffer * 100)

	// Determine chunk size (default 30, or 0 to disable chunking)
	chunkSize := o.cfg.Generation.SubtopicChunkSize
	if chunkSize == 0 {
		chunkSize = requestCount // Single request (old behavior)
	}

	// Log strategy
	if chunkSize < requestCount {
		o.logger.Info("Generating subtopics with chunking and over-generation strategy",
			"target", targetCount,
			"requesting", requestCount,
			"chunk_size", chunkSize,
			"num_chunks", (requestCount+chunkSize-1)/chunkSize,
			"buffer_percent", bufferPercent)
	} else {
		o.logger.Info("Generating subtopics with over-generation strategy (no chunking)",
			"target", targetCount,
			"requesting", requestCount,
			"buffer_percent", bufferPercent)
	}

	// Request subtopics in chunks
	var allSubtopics []string
	remaining := requestCount

	for remaining > 0 {
		currentChunk := chunkSize
		if currentChunk > remaining {
			currentChunk = remaining
		}

		o.logger.Debug("Requesting subtopic chunk",
			"chunk_size", currentChunk,
			"remaining", remaining,
			"collected", len(allSubtopics))

		chunkSubtopics, err := o.requestSubtopics(ctx, currentChunk, nil)
		if err != nil {
			if len(allSubtopics) > 0 {
				// Partial success - log warning and continue with what we have
				o.logger.Warn("Chunk request failed, continuing with partial results",
					"error", err,
					"collected_so_far", len(allSubtopics))
				break
			}
			return nil, fmt.Errorf("initial subtopic generation failed: %w", err)
		}

		allSubtopics = append(allSubtopics, chunkSubtopics...)
		remaining -= currentChunk
	}

	subtopics := allSubtopics

	// Deduplicate
	uniqueSubtopics := deduplicateStrings(subtopics)

	o.logger.Info("Initial subtopic generation complete",
		"requested", requestCount,
		"received", len(subtopics),
		"unique", len(uniqueSubtopics),
		"duplicates_filtered", len(subtopics)-len(uniqueSubtopics))

	// If we have enough, trim and return
	if len(uniqueSubtopics) >= targetCount {
		trimmed := uniqueSubtopics[:targetCount]
		o.logger.Info("Target count achieved",
			"final_count", len(trimmed),
			"excess_trimmed", len(uniqueSubtopics)-targetCount)
		return trimmed, nil
	}

	// If we're short, make ONE retry for the difference
	shortage := targetCount - len(uniqueSubtopics)
	o.logger.Warn("Subtopic count below target, attempting recovery",
		"current", len(uniqueSubtopics),
		"shortage", shortage)

	// Retry with exclusion list (but simpler prompt)
	retrySubtopics, retryErr := o.requestSubtopics(ctx, shortage, uniqueSubtopics)
	if retryErr != nil {
		o.logger.Warn("Retry failed, proceeding with partial results", "error", retryErr)
		return uniqueSubtopics, nil // Return what we have
	}

	// Merge and deduplicate again
	allSubtopics = append(uniqueSubtopics, retrySubtopics...)
	finalUnique := deduplicateStrings(allSubtopics)

	o.logger.Info("Subtopic generation complete after retry",
		"final_count", len(finalUnique),
		"target", targetCount,
		"success", len(finalUnique) >= targetCount)

	// Return what we have (may be less than target)
	if len(finalUnique) > targetCount {
		return finalUnique[:targetCount], nil
	}
	return finalUnique, nil
}

// truncateExclusionList limits the exclusion list to avoid prompt overflow
// Uses last N items as most recent failures are more relevant
// Returns the truncated list and a boolean indicating if truncation occurred
func truncateExclusionList(items []string, maxSize int) ([]string, bool) {
	if len(items) <= maxSize {
		return items, false
	}
	// Return last maxSize items (most recent)
	return items[len(items)-maxSize:], true
}

// requestSubtopics makes a single API call for subtopics
// exclusionList is optional (nil on first call, populated on retry)
func (o *Orchestrator) requestSubtopics(ctx context.Context, count int, exclusionList []string) ([]string, error) {
	// Build template data
	templateData := map[string]interface{}{
		"MainTopic":    o.cfg.Generation.MainTopic,
		"NumSubtopics": count,
		"IsRetry":      false, // Default to false
	}

	// Add exclusion list if present (for retry)
	if len(exclusionList) > 0 {
		// Truncate if necessary to prevent prompt overflow
		truncated, wasTruncated := truncateExclusionList(
			exclusionList,
			o.cfg.Generation.MaxExclusionListSize,
		)

		if wasTruncated {
			o.logger.Warn("Exclusion list truncated to prevent prompt overflow",
				"original_size", len(exclusionList),
				"truncated_size", len(truncated),
				"max_size", o.cfg.Generation.MaxExclusionListSize)
		}

		// Format as simple comma-separated list to keep LLM focused
		excluded := strings.Join(truncated, ", ")
		templateData["ExcludeSubtopics"] = excluded
		templateData["IsRetry"] = true
	}

	// Render template
	prompt, err := util.RenderTemplate(o.cfg.PromptTemplates.SubtopicGeneration, templateData)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	// Call API with structured output optimization
	mainModel := o.cfg.Models["main"]
	apiKey := o.secrets.GetAPIKey(mainModel.BaseURL)

	// Build messages with optional system prompt
	messages := []api.Message{}
	if o.cfg.PromptTemplates.SubtopicSystemPrompt != "" {
		messages = append(messages, api.Message{
			Role:    "system",
			Content: o.cfg.PromptTemplates.SubtopicSystemPrompt,
		})
	}
	messages = append(messages, api.Message{
		Role:    "user",
		Content: prompt,
	})

	resp, err := o.apiClient.ChatCompletionStructured(ctx, mainModel, apiKey, messages)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("API returned empty response")
	}
	content := resp.Choices[0].Message.Content
	o.logger.Debug("Received subtopics response", "length", len(content))

	// Extract and repair JSON
	jsonStr := extractJSON(content)
	o.logger.Debug("Extracted JSON", "length", len(jsonStr))

	jsonStr = util.RepairJSON(jsonStr)
	o.logger.Debug("Repaired JSON", "length", len(jsonStr))

	// Try validation first (advisory)
	valid, elemCount, err := ValidateJSONArray(jsonStr)
	if valid {
		o.logger.Debug("JSON validated successfully", "element_count", elemCount)
	} else {
		o.logger.Warn("JSON validation failed, attempting unmarshal anyway",
			"error", err,
			"extracted_json", util.TruncateString(jsonStr, 200))
	}

	// Attempt unmarshal with validation (with fallback to basic unmarshal)
	subtopics, actualCount, err := ValidateStringArray(jsonStr, 1)
	if err != nil {
		// Fallback: try basic unmarshal (old behavior)
		o.logger.Warn("ValidateStringArray failed, trying basic unmarshal", "error", err)
		var basicSubtopics []string
		if unmarshalErr := json.Unmarshal([]byte(jsonStr), &basicSubtopics); unmarshalErr != nil {
			o.logger.Error("Both validation and basic unmarshal failed",
				"validation_error", err,
				"unmarshal_error", unmarshalErr,
				"extracted_json", util.TruncateString(jsonStr, 200),
				"original_response", util.TruncateString(content, 200))
			return nil, fmt.Errorf("failed to parse subtopics: %w (unmarshal also failed: %v)", err, unmarshalErr)
		}
		subtopics = basicSubtopics
		actualCount = len(basicSubtopics)
		o.logger.Info("Basic unmarshal succeeded", "count", actualCount)
	}

	o.logger.Info("Subtopics parsed successfully",
		"requested", count,
		"received", actualCount)

	return subtopics, nil
}

func (o *Orchestrator) generatePrompts(ctx context.Context, subtopics []string) ([]models.GenerationJob, error) {
	o.logger.Info("Generating prompts for all subtopics with parallel workers...", "total_subtopics", len(subtopics), "concurrency", o.cfg.Generation.Concurrency)

	// Use worker pool for parallel prompt generation
	type subtopicTask struct {
		subtopic string
		index    int
	}

	type promptResult struct {
		index    int
		subtopic string
		prompts  []string
		err      error
	}

	tasksChan := make(chan subtopicTask, len(subtopics))
	resultsChan := make(chan promptResult, len(subtopics))

	// Start workers
	var wg sync.WaitGroup
	wg.Add(o.cfg.Generation.Concurrency) // Add all workers before starting goroutines
	for i := 0; i < o.cfg.Generation.Concurrency; i++ {
		go func(workerID int) {
			defer wg.Done()
			for task := range tasksChan {
				prompts, err := o.generatePromptsForSubtopic(ctx, task.subtopic)
				resultsChan <- promptResult{
					index:    task.index,
					subtopic: task.subtopic,
					prompts:  prompts,
					err:      err,
				}
			}
		}(i)
	}

	// Send tasks
	for i, subtopic := range subtopics {
		tasksChan <- subtopicTask{subtopic: subtopic, index: i}
	}
	close(tasksChan)

	// Wait for workers
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	results := make(map[int]promptResult)
	bar := progressbar.Default(int64(len(subtopics)), "Generating prompts")

	for result := range resultsChan {
		results[result.index] = result
		_ = bar.Add(1)

		if result.err != nil {
			o.logger.Error("Failed to generate prompts for subtopic",
				"subtopic", result.subtopic,
				"error", result.err)
		}
	}

	// Build jobs in order
	var allJobs []models.GenerationJob
	jobID := 0

	for i := 0; i < len(subtopics); i++ {
		result, ok := results[i]
		if !ok || result.err != nil {
			if ok {
				return nil, fmt.Errorf("failed to generate prompts for subtopic %q: %w", result.subtopic, result.err)
			}
			return nil, fmt.Errorf("missing result for subtopic at index %d", i)
		}

		for _, p := range result.prompts {
			allJobs = append(allJobs, models.GenerationJob{
				ID:        jobID,
				MainTopic: o.cfg.Generation.MainTopic,
				SubTopic:  result.subtopic,
				Prompt:    p,
			})
			jobID++
		}
	}

	return allJobs, nil
}

// generatePromptsForSubtopic generates prompts for a single subtopic
func (o *Orchestrator) generatePromptsForSubtopic(ctx context.Context, subtopic string) ([]string, error) {
	// Render template
	prompt, err := util.RenderTemplate(o.cfg.PromptTemplates.PromptGeneration, map[string]interface{}{
		"SubTopic":   subtopic,
		"NumPrompts": o.cfg.Generation.NumPromptsPerSubtopic,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render prompt template: %w", err)
	}

	// Call API
	mainModel := o.cfg.Models["main"]
	apiKey := o.secrets.GetAPIKey(mainModel.BaseURL)

	// Build messages with optional system prompt
	messages := []api.Message{}
	if o.cfg.PromptTemplates.PromptSystemPrompt != "" {
		messages = append(messages, api.Message{
			Role:    "system",
			Content: o.cfg.PromptTemplates.PromptSystemPrompt,
		})
	}
	messages = append(messages, api.Message{
		Role:    "user",
		Content: prompt,
	})

	resp, err := o.apiClient.ChatCompletionStructured(ctx, mainModel, apiKey, messages)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("API returned empty response")
	}
	// Parse JSON response
	content := resp.Choices[0].Message.Content

	o.logger.Debug("Received prompts response", "subtopic", subtopic, "length", len(content))

	// Extract JSON from potential markdown code blocks
	jsonStr := extractJSON(content)

	o.logger.Debug("Extracted JSON", "length", len(jsonStr), "first_100_chars", util.TruncateString(jsonStr, 100))

	// Repair common JSON issues from LLM responses
	jsonStr = util.RepairJSON(jsonStr)
	o.logger.Debug("Repaired JSON", "length", len(jsonStr), "first_100_chars", util.TruncateString(jsonStr, 100))

	// Try validation first (advisory)
	valid, elemCount, err := ValidateJSONArray(jsonStr)
	if valid {
		o.logger.Debug("Prompts JSON validated successfully", "subtopic", subtopic, "element_count", elemCount)
	} else {
		o.logger.Warn("JSON validation failed for prompts, attempting unmarshal anyway",
			"error", err,
			"subtopic", subtopic,
			"extracted_json", util.TruncateString(jsonStr, 200))
	}

	// Attempt unmarshal with validation (with fallback to basic unmarshal)
	prompts, actualCount, err := ValidateStringArray(jsonStr, 1)
	if err != nil {
		// Fallback: try basic unmarshal (old behavior)
		o.logger.Warn("ValidateStringArray failed for prompts, trying basic unmarshal",
			"error", err,
			"subtopic", subtopic)
		var basicPrompts []string
		if unmarshalErr := json.Unmarshal([]byte(jsonStr), &basicPrompts); unmarshalErr != nil {
			o.logger.Error("Both validation and basic unmarshal failed for prompts",
				"validation_error", err,
				"unmarshal_error", unmarshalErr,
				"subtopic", subtopic,
				"extracted_json", util.TruncateString(jsonStr, 200),
				"original_response", util.TruncateString(content, 200))
			return nil, fmt.Errorf("failed to parse prompts for subtopic %q: %w (unmarshal also failed: %v)", subtopic, err, unmarshalErr)
		}
		prompts = basicPrompts
		actualCount = len(basicPrompts)
		o.logger.Info("Basic unmarshal succeeded for prompts", "subtopic", subtopic, "count", actualCount)
	} else {
		o.logger.Debug("Prompts parsed successfully", "subtopic", subtopic, "count", actualCount)
	}

	return prompts, nil
}

func (o *Orchestrator) generatePreferencePairs(ctx context.Context, jobs []models.GenerationJob) error {
	o.logger.Info("Generating preference pairs", "total_jobs", len(jobs), "concurrency", o.cfg.Generation.Concurrency)

	// Create channels
	jobsChan := make(chan models.GenerationJob, len(jobs))
	resultsChan := make(chan models.GenerationResult, len(jobs))

	// Start workers
	var wg sync.WaitGroup
	wg.Add(o.cfg.Generation.Concurrency) // Add all workers before starting goroutines
	for i := 0; i < o.cfg.Generation.Concurrency; i++ {
		go o.worker(ctx, i, jobsChan, resultsChan, &wg)
	}

	// Send jobs
	for _, job := range jobs {
		jobsChan <- job
	}
	close(jobsChan)

	// Start result collector
	var collectorWg sync.WaitGroup
	collectorWg.Add(1)
	go o.collectResults(resultsChan, &collectorWg)

	// Wait for workers to finish
	wg.Wait()
	close(resultsChan)

	// Wait for collector to finish
	collectorWg.Wait()

	return nil
}

// GetStats returns the session statistics
func (o *Orchestrator) GetStats() *models.SessionStats {
	return o.stats
}
