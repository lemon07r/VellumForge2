package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/lamim/vellumforge2/internal/api"
	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/internal/util"
	"github.com/lamim/vellumforge2/pkg/models"
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

	// Call judge model
	judgeModel := j.cfg.Models["judge"]
	apiKey := j.secrets.GetAPIKey(judgeModel.BaseURL)

	resp, err := j.apiClient.ChatCompletion(ctx, judgeModel, apiKey, []api.Message{
		{Role: "user", Content: judgePrompt},
	})
	if err != nil {
		return nil, err
	}

	// Parse response
	content := resp.Choices[0].Message.Content

	j.logger.Debug("Received judge response", "length", len(content), "first_200_chars", truncateString(content, 200))

	scores, err := j.parseJudgeResponse(content)
	if err != nil {
		j.logger.Error("Failed to parse judge response",
			"error", err,
			"response_length", len(content),
			"response", content)
		return nil, fmt.Errorf("failed to parse judge response: %w (response: %s)", err, content)
	}

	return scores, nil
}

func (j *Judge) parseJudgeResponse(response string) (map[string]models.CriteriaScore, error) {
	// Extract JSON from response (may be wrapped in markdown code blocks)
	jsonStr := extractJSON(response)

	j.logger.Debug("Extracted judge JSON", "length", len(jsonStr), "first_200_chars", truncateString(jsonStr, 200))

	// Sanitize JSON to handle common LLM issues
	jsonStr = sanitizeJSON(jsonStr)

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

// extractJSON extracts JSON content from a response that may contain markdown
// and properly handles nested structures using bracket matching
func extractJSON(s string) string {
	// Try to extract from markdown code blocks
	re := regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)```")
	matches := re.FindStringSubmatch(s)
	if len(matches) > 1 {
		s = strings.TrimSpace(matches[1])
	} else {
		s = strings.TrimSpace(s)
	}

	// Try to find JSON object boundaries using proper bracket matching
	objectStart := strings.Index(s, "{")
	if objectStart != -1 {
		// Find the matching closing brace by counting braces
		braceCount := 0
		inString := false
		escaped := false
		objectEnd := -1

		for i := objectStart; i < len(s); i++ {
			ch := s[i]

			// Handle escape sequences
			if escaped {
				escaped = false
				continue
			}

			if ch == '\\' {
				escaped = true
				continue
			}

			// Handle strings
			if ch == '"' {
				inString = !inString
				continue
			}

			// Only count braces outside of strings
			if !inString {
				if ch == '{' {
					braceCount++
				} else if ch == '}' {
					braceCount--
					if braceCount == 0 {
						objectEnd = i
						break
					}
				}
			}
		}

		if objectEnd != -1 {
			return s[objectStart : objectEnd+1]
		} else {
			// Truncated object - try to close it
			lastQuote := strings.LastIndex(s, "\"")
			if lastQuote > objectStart {
				// Has content, close the object
				trimmed := strings.TrimRight(s[objectStart:], " \n\t,")
				return trimmed + "}"
			}
		}
	}

	// Return as-is if no extraction needed
	return s
}

func calculateAverageScore(scores map[string]models.CriteriaScore) float64 {
	if len(scores) == 0 {
		return 0
	}

	sum := 0
	for _, score := range scores {
		sum += score.Score
	}

	return float64(sum) / float64(len(scores))
}

// sanitizeJSON fixes common JSON issues from LLM responses
func sanitizeJSON(s string) string {
	// Replace literal unescaped newlines in string values with escaped newlines
	// This handles cases where LLMs generate reasoning text with actual newlines
	var result strings.Builder
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escaped {
			result.WriteByte(ch)
			escaped = false
			continue
		}

		if ch == '\\' {
			result.WriteByte(ch)
			escaped = true
			continue
		}

		if ch == '"' {
			result.WriteByte(ch)
			inString = !inString
			continue
		}

		// Replace literal newlines in strings with \n
		if inString && (ch == '\n' || ch == '\r') {
			result.WriteString("\\n")
			// Skip \r if followed by \n
			if ch == '\r' && i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			continue
		}

		result.WriteByte(ch)
	}

	return result.String()
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
