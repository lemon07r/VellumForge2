package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lamim/vellumforge2/internal/api"
	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/internal/util"
	"github.com/lamim/vellumforge2/pkg/models"
)

const (
	// maxParseStrategies is the number of different parsing strategies to try
	// Each strategy uses different repair techniques on the SAME API response
	maxParseStrategies = 4
)

// Judge handles LLM-as-a-Judge evaluations
type Judge struct {
	cfg       *config.Config
	secrets   *config.Secrets
	apiClient *api.Client
	logger    *slog.Logger
}

// New creates a new judge
func New(cfg *config.Config, secrets *config.Secrets, apiClient *api.Client, logger *slog.Logger) *Judge {
	return &Judge{
		cfg:       cfg,
		secrets:   secrets,
		apiClient: apiClient,
		logger:    logger.With("component", "judge"),
	}
}

// Evaluate sends a story to the judge model for evaluation
func (j *Judge) Evaluate(ctx context.Context, prompt, chosen, rejected string) (*models.JudgeResult, error) {
	// Evaluate chosen response
	chosenScores, err := j.evaluateSingle(ctx, prompt, chosen)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate chosen response: %w", err)
	}

	// Evaluate rejected response
	rejectedScores, err := j.evaluateSingle(ctx, prompt, rejected)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate rejected response: %w", err)
	}

	// Calculate total scores (average across all criteria)
	chosenScoreTotal := calculateAverageScore(chosenScores)
	rejectedScoreTotal := calculateAverageScore(rejectedScores)
	preferenceMargin := chosenScoreTotal - rejectedScoreTotal

	return &models.JudgeResult{
		ChosenScores:       chosenScores,
		RejectedScores:     rejectedScores,
		ChosenScoreTotal:   chosenScoreTotal,
		RejectedScoreTotal: rejectedScoreTotal,
		PreferenceMargin:   preferenceMargin,
	}, nil
}

func (j *Judge) evaluateSingle(ctx context.Context, prompt, story string) (map[string]models.CriteriaScore, error) {
	// Render judge prompt
	judgePrompt, err := util.RenderTemplate(j.cfg.PromptTemplates.JudgeRubric, map[string]interface{}{
		"Prompt":    prompt,
		"StoryText": story,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render judge template: %w", err)
	}

	judgeModel := j.cfg.Models["judge"]
	apiKey := j.secrets.GetAPIKey(judgeModel.BaseURL)

	// Create timeout context for judge API call
	// Use configured timeout (default: 100s, generous for slower models)
	timeoutDuration := time.Duration(judgeModel.JudgeTimeoutSeconds) * time.Second
	timeoutCtx, cancel := context.WithTimeout(ctx, timeoutDuration)
	defer cancel()

	// Call judge model ONCE
	// API-level retries are handled by the API client for network errors, timeouts, etc.
	resp, err := j.apiClient.ChatCompletion(timeoutCtx, judgeModel, apiKey, []api.Message{
		{Role: "user", Content: judgePrompt},
	})
	if err != nil {
		// API call failed - network error, timeout, rate limit, etc.
		// The API client has already retried these errors appropriately
		return nil, fmt.Errorf("API call failed: %w", err)
	}

	// Parse response with multiple strategies
	// This tries different JSON repair techniques on the SAME response
	// No additional API calls are made - this is purely local processing
	content := resp.Choices[0].Message.Content
	j.logger.Debug("Received judge response",
		"length", len(content),
		"first_200_chars", truncateString(content, 200))

	scores, err := j.parseJudgeResponseWithRetries(content)
	if err != nil {
		j.logger.Error("Failed to parse judge response after all strategies",
			"error", err,
			"response_length", len(content),
			"response", truncateString(content, 1000))
		return nil, fmt.Errorf("failed to parse judge response: %w", err)
	}

	return scores, nil
}

// parseJudgeResponseWithRetries tries multiple JSON parsing strategies on the same response
// This allows us to recover from common LLM JSON issues without making additional API calls
func (j *Judge) parseJudgeResponseWithRetries(response string) (map[string]models.CriteriaScore, error) {
	var lastErr error
	
	// Strategy 1: Standard extraction + sanitization
	// This is the most common case and should work ~95% of the time
	if scores, err := j.parseJudgeResponseStrategy1(response); err == nil {
		j.logger.Debug("Parse succeeded with strategy 1 (standard)",
			"response_length", len(response))
		return scores, nil
	} else {
		lastErr = err
		j.logger.Debug("Parse strategy 1 failed",
			"error", err,
			"will_try_next_strategy", true)
	}

	// Strategy 2: Aggressive repair with RepairJSON
	// This handles missing commas, trailing commas, unescaped quotes, etc.
	if scores, err := j.parseJudgeResponseStrategy2(response); err == nil {
		j.logger.Info("Parse succeeded with strategy 2 (aggressive repair)",
			"response_length", len(response))
		return scores, nil
	} else {
		lastErr = err
		j.logger.Debug("Parse strategy 2 failed",
			"error", err,
			"will_try_next_strategy", true)
	}

	// Strategy 3: Extract and repair with multiple passes
	// This tries extraction first, then applies multiple repair techniques
	if scores, err := j.parseJudgeResponseStrategy3(response); err == nil {
		j.logger.Info("Parse succeeded with strategy 3 (multi-pass repair)",
			"response_length", len(response))
		return scores, nil
	} else {
		lastErr = err
		j.logger.Debug("Parse strategy 3 failed",
			"error", err,
			"will_try_next_strategy", true)
	}

	// Strategy 4: Partial JSON recovery
	// This attempts to extract and parse any valid JSON object, even if incomplete
	if scores, err := j.parseJudgeResponseStrategy4(response); err == nil {
		j.logger.Warn("Parse succeeded with strategy 4 (partial recovery)",
			"response_length", len(response),
			"note", "may have incomplete data")
		return scores, nil
	} else {
		lastErr = err
		j.logger.Debug("Parse strategy 4 failed",
			"error", err,
			"all_strategies_exhausted", true)
	}

	// All strategies failed
	return nil, fmt.Errorf("all %d parse strategies failed, last error: %w", maxParseStrategies, lastErr)
}

// parseJudgeResponseStrategy1 uses standard extraction + sanitization
func (j *Judge) parseJudgeResponseStrategy1(response string) (map[string]models.CriteriaScore, error) {
	// Extract JSON from response (may be wrapped in markdown code blocks)
	jsonStr := util.ExtractJSON(response)

	j.logger.Debug("Strategy 1: Extracted JSON", 
		"length", len(jsonStr), 
		"first_200_chars", truncateString(jsonStr, 200))

	// Sanitize JSON to handle common LLM issues
	jsonStr = util.SanitizeJSON(jsonStr)

	// Parse the JSON
	var rawScores map[string]models.CriteriaScore
	if err := json.Unmarshal([]byte(jsonStr), &rawScores); err != nil {
		return nil, fmt.Errorf("strategy 1 unmarshal failed: %w", err)
	}

	return rawScores, nil
}

// parseJudgeResponseStrategy2 uses aggressive repair with RepairJSON
func (j *Judge) parseJudgeResponseStrategy2(response string) (map[string]models.CriteriaScore, error) {
	// First extract, then apply aggressive repair
	jsonStr := util.ExtractJSON(response)
	jsonStr = util.RepairJSON(jsonStr)

	j.logger.Debug("Strategy 2: Repaired JSON",
		"length", len(jsonStr),
		"first_200_chars", truncateString(jsonStr, 200))

	var rawScores map[string]models.CriteriaScore
	if err := json.Unmarshal([]byte(jsonStr), &rawScores); err != nil {
		return nil, fmt.Errorf("strategy 2 unmarshal failed: %w", err)
	}

	return rawScores, nil
}

// parseJudgeResponseStrategy3 uses multiple passes of repair
func (j *Judge) parseJudgeResponseStrategy3(response string) (map[string]models.CriteriaScore, error) {
	// Apply repairs in sequence: Extract -> Repair -> Sanitize -> Repair again
	jsonStr := util.ExtractJSON(response)
	jsonStr = util.RepairJSON(jsonStr)
	jsonStr = util.SanitizeJSON(jsonStr)
	jsonStr = util.RepairJSON(jsonStr) // Second pass to fix issues introduced by sanitization

	j.logger.Debug("Strategy 3: Multi-pass repaired JSON",
		"length", len(jsonStr),
		"first_200_chars", truncateString(jsonStr, 200))

	var rawScores map[string]models.CriteriaScore
	if err := json.Unmarshal([]byte(jsonStr), &rawScores); err != nil {
		return nil, fmt.Errorf("strategy 3 unmarshal failed: %w", err)
	}

	return rawScores, nil
}

// parseJudgeResponseStrategy4 attempts partial JSON recovery
func (j *Judge) parseJudgeResponseStrategy4(response string) (map[string]models.CriteriaScore, error) {
	// Try to find and extract any valid JSON object, even if incomplete
	jsonStr := util.ExtractJSON(response)
	
	// If extraction gave us something, try to parse it even if potentially incomplete
	if len(jsonStr) > 0 {
		// Apply all repairs
		jsonStr = util.RepairJSON(jsonStr)
		jsonStr = util.SanitizeJSON(jsonStr)
		
		// Try to parse with a more lenient decoder
		var rawScores map[string]models.CriteriaScore
		decoder := json.NewDecoder(strings.NewReader(jsonStr))
		
		// Attempt to decode - this may give us partial data
		if err := decoder.Decode(&rawScores); err != nil {
			return nil, fmt.Errorf("strategy 4 decode failed: %w", err)
		}

		// If we got at least some scores, return them
		if len(rawScores) > 0 {
			j.logger.Debug("Strategy 4: Partial recovery succeeded",
				"scores_recovered", len(rawScores))
			return rawScores, nil
		}
	}

	return nil, fmt.Errorf("strategy 4 failed: no valid JSON found")
}

// parseJudgeResponse is kept for backward compatibility and simplicity
// It now uses the standard strategy
func (j *Judge) parseJudgeResponse(response string) (map[string]models.CriteriaScore, error) {
	return j.parseJudgeResponseStrategy1(response)
}

func calculateAverageScore(scores map[string]models.CriteriaScore) float64 {
	if len(scores) == 0 {
		return 0.0
	}

	sum := 0
	for _, score := range scores {
		sum += score.Score
	}

	return float64(sum) / float64(len(scores))
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
