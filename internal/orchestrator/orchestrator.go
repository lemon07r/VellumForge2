package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lamim/vellumforge2/internal/api"
	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/internal/judge"
	"github.com/lamim/vellumforge2/internal/util"
	"github.com/lamim/vellumforge2/internal/writer"
	"github.com/lamim/vellumforge2/pkg/models"
	"github.com/schollz/progressbar/v3"
)

// Orchestrator manages the entire data generation pipeline
type Orchestrator struct {
	cfg         *config.Config
	secrets     *config.Secrets
	apiClient   *api.Client
	judgeModule *judge.Judge
	dataWriter  *writer.DatasetWriter
	logger      *slog.Logger
	stats       *models.SessionStats
}

// New creates a new orchestrator
func New(
	cfg *config.Config,
	secrets *config.Secrets,
	apiClient *api.Client,
	dataWriter *writer.DatasetWriter,
	logger *slog.Logger,
) *Orchestrator {
	var judgeModule *judge.Judge
	if judgeModel, ok := cfg.Models["judge"]; ok && judgeModel.Enabled {
		judgeModule = judge.New(cfg, secrets, apiClient, logger)
	}

	return &Orchestrator{
		cfg:         cfg,
		secrets:     secrets,
		apiClient:   apiClient,
		judgeModule: judgeModule,
		dataWriter:  dataWriter,
		logger:      logger,
		stats: &models.SessionStats{
			StartTime: time.Now(),
		},
	}
}

// Run executes the complete generation pipeline
func (o *Orchestrator) Run(ctx context.Context) error {
	o.logger.Info("Starting generation pipeline",
		"main_topic", o.cfg.Generation.MainTopic,
		"num_subtopics", o.cfg.Generation.NumSubtopics,
		"prompts_per_subtopic", o.cfg.Generation.NumPromptsPerSubtopic)

	// Phase 1: Generate subtopics
	subtopics, err := o.generateSubtopics(ctx)
	if err != nil {
		return fmt.Errorf("failed to generate subtopics: %w", err)
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
	prompts, err := o.generatePrompts(ctx, subtopics)
	if err != nil {
		return fmt.Errorf("failed to generate prompts: %w", err)
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

	// Phase 3: Generate preference pairs concurrently
	if err := o.generatePreferencePairs(ctx, prompts); err != nil {
		return fmt.Errorf("failed to generate preference pairs: %w", err)
	}

	// Finalize stats
	o.stats.EndTime = time.Now()
	o.stats.TotalDuration = o.stats.EndTime.Sub(o.stats.StartTime)
	if o.stats.SuccessCount > 0 {
		o.stats.AverageDuration = o.stats.TotalDuration / time.Duration(o.stats.SuccessCount)
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

	return nil
}

func (o *Orchestrator) generateSubtopics(ctx context.Context) ([]string, error) {
	o.logger.Info("Generating subtopics...")

	// Render template
	prompt, err := util.RenderTemplate(o.cfg.PromptTemplates.SubtopicGeneration, map[string]interface{}{
		"MainTopic":    o.cfg.Generation.MainTopic,
		"NumSubtopics": o.cfg.Generation.NumSubtopics,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render subtopic template: %w", err)
	}

	// Call API
	mainModel := o.cfg.Models["main"]
	apiKey := o.secrets.GetAPIKey(mainModel.BaseURL)

	resp, err := o.apiClient.ChatCompletion(ctx, mainModel, apiKey, []api.Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	content := resp.Choices[0].Message.Content

	o.logger.Debug("Received subtopics response", "length", len(content))

	// Extract JSON from potential markdown code blocks
	jsonStr := extractJSON(content)

	o.logger.Debug("Extracted JSON", "length", len(jsonStr), "json", jsonStr)

	var subtopics []string
	if err := json.Unmarshal([]byte(jsonStr), &subtopics); err != nil {
		o.logger.Error("Failed to parse subtopics JSON", "error", err, "extracted_json", jsonStr, "original_response", content)
		return nil, fmt.Errorf("failed to parse subtopics JSON: %w (response: %s)", err, content)
	}

	return subtopics, nil
}

func (o *Orchestrator) generatePrompts(ctx context.Context, subtopics []string) ([]models.GenerationJob, error) {
	o.logger.Info("Generating prompts for all subtopics...")

	var allJobs []models.GenerationJob
	jobID := 0

	bar := progressbar.Default(int64(len(subtopics)), "Generating prompts")

	for _, subtopic := range subtopics {
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

		resp, err := o.apiClient.ChatCompletion(ctx, mainModel, apiKey, []api.Message{
			{Role: "user", Content: prompt},
		})
		if err != nil {
			return nil, err
		}

		// Parse JSON response
		content := resp.Choices[0].Message.Content

		o.logger.Debug("Received prompts response", "subtopic", subtopic, "length", len(content))

		// Extract JSON from potential markdown code blocks
		jsonStr := extractJSON(content)

		o.logger.Debug("Extracted JSON", "length", len(jsonStr), "first_100_chars", truncateString(jsonStr, 100))

		var prompts []string
		if err := json.Unmarshal([]byte(jsonStr), &prompts); err != nil {
			o.logger.Error("Failed to parse prompts JSON",
				"error", err,
				"subtopic", subtopic,
				"extracted_json_length", len(jsonStr),
				"extracted_json", jsonStr,
				"original_response_length", len(content),
				"original_response", content)
			return nil, fmt.Errorf("failed to parse prompts JSON: %w (response: %s)", err, content)
		}

		// Create jobs
		for _, p := range prompts {
			allJobs = append(allJobs, models.GenerationJob{
				ID:        jobID,
				MainTopic: o.cfg.Generation.MainTopic,
				SubTopic:  subtopic,
				Prompt:    p,
			})
			jobID++
		}

		_ = bar.Add(1)
	}

	return allJobs, nil
}

func (o *Orchestrator) generatePreferencePairs(ctx context.Context, jobs []models.GenerationJob) error {
	o.logger.Info("Generating preference pairs", "total_jobs", len(jobs), "concurrency", o.cfg.Generation.Concurrency)

	// Create channels
	jobsChan := make(chan models.GenerationJob, len(jobs))
	resultsChan := make(chan models.GenerationResult, len(jobs))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < o.cfg.Generation.Concurrency; i++ {
		wg.Add(1)
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

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
