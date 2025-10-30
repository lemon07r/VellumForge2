package judge

import (
	"log/slog"
	"testing"

	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/pkg/models"
)

func TestParseJudgeResponse_ValidJSON(t *testing.T) {
	j := setupTestJudge()

	response := `{"creativity": {"score": 8, "reasoning": "Good use of imagination"}}`
	scores, err := j.parseJudgeResponse(response)

	if err != nil {
		t.Fatalf("parseJudgeResponse returned unexpected error: %v", err)
	}

	if len(scores) != 1 {
		t.Errorf("Expected 1 score, got %d", len(scores))
	}

	creativity, ok := scores["creativity"]
	if !ok {
		t.Fatal("Expected 'creativity' score not found")
	}

	if creativity.Score != 8 {
		t.Errorf("Expected score 8, got %d", creativity.Score)
	}

	if creativity.Reasoning != "Good use of imagination" {
		t.Errorf("Expected reasoning 'Good use of imagination', got %q", creativity.Reasoning)
	}
}

func TestParseJudgeResponse_MultipleScores(t *testing.T) {
	j := setupTestJudge()

	response := `{
		"creativity": {"score": 8, "reasoning": "Imaginative"},
		"coherence": {"score": 7, "reasoning": "Logical flow"},
		"relevance": {"score": 9, "reasoning": "On topic"}
	}`
	scores, err := j.parseJudgeResponse(response)

	if err != nil {
		t.Fatalf("parseJudgeResponse returned unexpected error: %v", err)
	}

	if len(scores) != 3 {
		t.Errorf("Expected 3 scores, got %d", len(scores))
	}

	expected := map[string]int{
		"creativity": 8,
		"coherence":  7,
		"relevance":  9,
	}

	for criterion, expectedScore := range expected {
		score, ok := scores[criterion]
		if !ok {
			t.Errorf("Expected criterion %q not found", criterion)
			continue
		}
		if score.Score != expectedScore {
			t.Errorf("Criterion %q: expected score %d, got %d", criterion, expectedScore, score.Score)
		}
	}
}

func TestParseJudgeResponse_MarkdownWrapped(t *testing.T) {
	j := setupTestJudge()

	// LLMs often wrap JSON in markdown code blocks
	response := "```json\n{\"creativity\": {\"score\": 8, \"reasoning\": \"Good\"}}\n```"
	scores, err := j.parseJudgeResponse(response)

	if err != nil {
		t.Fatalf("parseJudgeResponse with markdown returned unexpected error: %v", err)
	}

	if len(scores) != 1 {
		t.Errorf("Expected 1 score, got %d", len(scores))
	}

	creativity, ok := scores["creativity"]
	if !ok {
		t.Fatal("Expected 'creativity' score not found")
	}

	if creativity.Score != 8 {
		t.Errorf("Expected score 8, got %d", creativity.Score)
	}
}

func TestParseJudgeResponse_InvalidJSON(t *testing.T) {
	j := setupTestJudge()

	tests := []struct {
		name     string
		response string
	}{
		{
			name:     "incomplete",
			response: `{"creativity": {"score"`,
		},
		{
			name:     "malformed",
			response: "not json at all",
		},
		{
			name:     "empty",
			response: "",
		},
		{
			name:     "just_text",
			response: "This is just a text response without any JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := j.parseJudgeResponse(tt.response)
			if err == nil {
				t.Errorf("Expected error for invalid JSON %q, got nil", tt.response)
			}
		})
	}
}

// Note: Full integration tests for Evaluate() would require mocking the API client
// which would need refactoring Judge to use an interface. For now, we test the
// public functions that don't require API calls.

func TestCalculateAverageScore_EmptyMap(t *testing.T) {
	scores := map[string]models.CriteriaScore{}
	avg := calculateAverageScore(scores)

	if avg != 0.0 {
		t.Errorf("Expected average 0.0 for empty map, got %f", avg)
	}
}

func TestCalculateAverageScore_SingleScore(t *testing.T) {
	scores := map[string]models.CriteriaScore{
		"creativity": {Score: 8, Reasoning: "Good"},
	}
	avg := calculateAverageScore(scores)

	if avg != 8.0 {
		t.Errorf("Expected average 8.0, got %f", avg)
	}
}

func TestCalculateAverageScore_MultipleScores(t *testing.T) {
	scores := map[string]models.CriteriaScore{
		"creativity": {Score: 8, Reasoning: "Good"},
		"coherence":  {Score: 6, Reasoning: "Fair"},
		"relevance":  {Score: 10, Reasoning: "Excellent"},
	}
	avg := calculateAverageScore(scores)

	expected := 8.0 // (8+6+10)/3 = 24/3 = 8
	if avg != expected {
		t.Errorf("Expected average %.1f, got %.1f", expected, avg)
	}
}

func TestCalculateAverageScore_ZeroScores(t *testing.T) {
	scores := map[string]models.CriteriaScore{
		"creativity": {Score: 0, Reasoning: "Poor"},
		"coherence":  {Score: 0, Reasoning: "Poor"},
	}
	avg := calculateAverageScore(scores)

	if avg != 0.0 {
		t.Errorf("Expected average 0.0, got %f", avg)
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "shorter_than_max",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "equal_to_max",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "longer_than_max",
			input:    "hello world",
			maxLen:   5,
			expected: "hello...",
		},
		{
			name:     "empty_string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

// Helper function to create a test judge instance
func setupTestJudge() *Judge {
	return &Judge{
		cfg:     &config.Config{},
		secrets: &config.Secrets{},
		logger:  slog.Default(),
	}
}
