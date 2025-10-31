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
	// maxParseRetries is the maximum number of retry attempts for JSON parse errors
	// This handles cases where API returns truncated or malformed JSON
	maxParseRetries = 2
	// parseRetryDelay is the delay between parse retry attempts
	parseRetryDelay = 2 * time.Second
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

	// Retry loop for API call + parse
	// This handles transient issues like truncated JSON responses
	var lastErr error
	for attempt := 0; attempt <= maxParseRetries; attempt++ {
		// Add delay before retry (not on first attempt)
		if attempt > 0 {
			j.logger.Warn("Retrying judge evaluation after parse failure",
				"attempt", attempt,
				"max_retries", maxParseRetries,
				"delay", parseRetryDelay,
				"last_error", lastErr)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(parseRetryDelay):
			}
		}

		// Create timeout context for judge API call
		// Use configured timeout (default: 100s, generous for slower models)
		timeoutDuration := time.Duration(judgeModel.JudgeTimeoutSeconds) * time.Second
		timeoutCtx, cancel := context.WithTimeout(ctx, timeoutDuration)

		// Call judge model
		resp, err := j.apiClient.ChatCompletion(timeoutCtx, judgeModel, apiKey, []api.Message{
			{Role: "user", Content: judgePrompt},
		})
		cancel() // Clean up timeout context

		if err != nil {
			// API call failed - don't retry parse errors, let API retry logic handle it
			return nil, err
		}

		// Parse response
		content := resp.Choices[0].Message.Content

		j.logger.Debug("Received judge response",
			"attempt", attempt+1,
			"length", len(content),
			"first_200_chars", truncateString(content, 200))

		scores, err := j.parseJudgeResponse(content)
		if err == nil {
			// Success!
			if attempt > 0 {
				j.logger.Info("Judge evaluation succeeded after retry",
					"attempt", attempt+1,
					"total_attempts", attempt+1)
			}
			return scores, nil
		}

		// Parse failed
		lastErr = err

		// Check if error is retryable (JSON parse errors)
		if !isJSONParseError(err) {
			// Non-retryable error (shouldn't happen, but handle it)
			j.logger.Error("Non-retryable parse error",
				"error", err,
				"response_length", len(content))
			return nil, fmt.Errorf("failed to parse judge response: %w (response: %s)", err, content)
		}

		// Retryable error - log and continue to next attempt
		j.logger.Warn("Judge response parse failed (retryable)",
			"attempt", attempt+1,
			"error", err,
			"response_length", len(content),
			"will_retry", attempt < maxParseRetries)

		// If this was the last attempt, log full response for debugging
		if attempt == maxParseRetries {
			j.logger.Error("Judge evaluation failed after all retries",
				"total_attempts", maxParseRetries+1,
				"error", lastErr,
				"response_length", len(content),
				"response", content)
			return nil, fmt.Errorf("failed to parse judge response after %d attempts: %w (response: %s)", 
				maxParseRetries+1, lastErr, truncateString(content, 500))
		}
	}

	// Should never reach here, but handle it
	return nil, fmt.Errorf("failed to evaluate judge response after %d attempts: %w", maxParseRetries+1, lastErr)
}

func (j *Judge) parseJudgeResponse(response string) (map[string]models.CriteriaScore, error) {
	// Extract JSON from response (may be wrapped in markdown code blocks)
	jsonStr := util.ExtractJSON(response)

	j.logger.Debug("Extracted judge JSON", "length", len(jsonStr), "first_200_chars", truncateString(jsonStr, 200))

	// Sanitize JSON to handle common LLM issues
	jsonStr = util.SanitizeJSON(jsonStr)

	// Parse the JSON
	var rawScores map[string]models.CriteriaScore
	if err := json.Unmarshal([]byte(jsonStr), &rawScores); err != nil {
		j.logger.Error("Failed to unmarshal judge JSON",
			"error", err,
			"extracted_json_length", len(jsonStr),
			"extracted_json", jsonStr)
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// Note: We accept any criteria the model returns
	// No longer validating against a fixed list to allow flexibility

	return rawScores, nil
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

// isJSONParseError checks if an error is a JSON parsing error that should be retried
func isJSONParseError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Check for common JSON parse error patterns
	return strings.Contains(errStr, "unexpected end of JSON input") ||
		strings.Contains(errStr, "failed to unmarshal JSON") ||
		strings.Contains(errStr, "invalid character") ||
		strings.Contains(errStr, "JSON decode error")
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
