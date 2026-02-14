package api

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lamim/vellumforge2/internal/config"
)

// TestReasoningCapabilities tests the reasoning model with prompts that require logical thinking
func TestReasoningCapabilities(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Load config to get model settings
	cfg, _, err := config.Load("../../config.sft.toml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create API client
	client := NewClient(logger)
	client.SetMaxRetries(2)

	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping reasoning test")
	}

	// Define test cases that require reasoning
	testCases := []struct {
		name             string
		prompt           string
		expectedKeywords []string // Keywords we expect in a good reasoning response
	}{
		{
			name: "Math Reasoning",
			prompt: `Solve this step by step: If a train travels 120 miles in 2 hours, then stops for 30 minutes, 
then continues for another 90 miles at the same speed, how long did the entire journey take?`,
			expectedKeywords: []string{"speed", "60", "1.5", "hour", "3", "total"},
		},
		{
			name: "Logic Puzzle",
			prompt: `Three people are standing in a line. Alice is not at the front. Bob is not at the back. 
Carol is behind Alice. What is the order from front to back?`,
			expectedKeywords: []string{"Bob", "Alice", "Carol", "order"},
		},
		{
			name:             "Pattern Recognition",
			prompt:           `What is the next number in this sequence and why: 2, 6, 12, 20, 30, ?`,
			expectedKeywords: []string{"42", "pattern", "difference"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			messages := []Message{
				{
					Role:    "user",
					Content: tc.prompt,
				},
			}

			t.Logf("Testing reasoning with prompt: %s", tc.prompt)

			mainModel := cfg.Models["main"]
			resp, err := client.ChatCompletion(ctx, mainModel, apiKey, messages)
			if err != nil {
				t.Fatalf("ChatCompletion failed: %v", err)
			}

			if len(resp.Choices) == 0 {
				t.Fatal("No choices returned in response")
			}

			response := resp.Choices[0].Message.Content
			t.Logf("Model: %s", resp.Model)
			t.Logf("Response length: %d characters", len(response))
			t.Logf("Response:\n%s", response)

			// Check if response contains expected keywords (case-insensitive)
			responseUpper := strings.ToUpper(response)
			foundKeywords := 0
			for _, keyword := range tc.expectedKeywords {
				if strings.Contains(responseUpper, strings.ToUpper(keyword)) {
					foundKeywords++
				}
			}

			// We expect at least half of the keywords to be present
			if foundKeywords < len(tc.expectedKeywords)/2 {
				t.Logf("WARNING: Response may not contain sufficient reasoning. Found %d/%d expected keywords",
					foundKeywords, len(tc.expectedKeywords))
			}

			// Check that response is not empty and has reasonable length
			if len(response) < 50 {
				t.Errorf("Response too short (%d chars), expected detailed reasoning", len(response))
			}

			// Check token usage
			if resp.Usage.TotalTokens > 0 {
				t.Logf("Token usage - Prompt: %d, Completion: %d, Total: %d",
					resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
			}

			// Check finish reason
			if resp.Choices[0].FinishReason == "length" {
				t.Logf("WARNING: Response was truncated (finish_reason: length)")
			}
		})
	}
}

// TestReasoningWithSystemPrompt tests reasoning with a system prompt
func TestReasoningWithSystemPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, _, err := config.Load("../../config.sft.toml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	client := NewClient(logger)
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping reasoning test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	messages := []Message{
		{
			Role: "system",
			Content: `You are an expert mathematician. When solving problems, always show your step-by-step 
reasoning process. Break down complex problems into simpler steps.`,
		},
		{
			Role: "user",
			Content: `Calculate: (15 + 8) × 3 - 12 ÷ 4
Show your work step by step.`,
		},
	}

	t.Log("Testing reasoning with system prompt for explicit step-by-step guidance")

	mainModel := cfg.Models["main"]
	resp, err := client.ChatCompletion(ctx, mainModel, apiKey, messages)
	if err != nil {
		t.Fatalf("ChatCompletion failed: %v", err)
	}

	response := resp.Choices[0].Message.Content
	t.Logf("Response:\n%s", response)

	// Check for step-by-step reasoning markers
	hasSteps := strings.Contains(strings.ToLower(response), "step") ||
		strings.Contains(response, "1.") ||
		strings.Contains(response, "first") ||
		strings.Contains(response, "then")

	if !hasSteps {
		t.Log("WARNING: Response may not show explicit step-by-step reasoning")
	}

	// Check for the correct final answer (66)
	if strings.Contains(response, "66") {
		t.Log("✓ Correct answer (66) found in response")
	} else {
		t.Log("WARNING: Expected answer (66) not clearly present in response")
	}
}

// TestReasoningStructuredOutput tests reasoning with structured JSON output
func TestReasoningStructuredOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, _, err := config.Load("../../config.sft.toml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	client := NewClient(logger)
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping reasoning test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	messages := []Message{
		{
			Role: "user",
			Content: `Analyze this problem and provide your reasoning in JSON format:

Problem: A farmer has chickens and cows. There are 30 heads and 74 legs total. How many chickens and cows are there?

Return ONLY a JSON object with this structure:
{
  "problem_understanding": "brief summary of the problem",
  "reasoning_steps": ["step 1", "step 2", "step 3"],
  "solution": {
    "chickens": number,
    "cows": number
  },
  "verification": "how to verify the answer"
}`,
		},
	}

	t.Log("Testing reasoning with structured JSON output requirement")

	mainModel := cfg.Models["main"]
	// Use ChatCompletionStructured which applies structure_temperature if configured
	resp, err := client.ChatCompletionStructured(ctx, mainModel, apiKey, messages)
	if err != nil {
		t.Fatalf("ChatCompletion failed: %v", err)
	}

	response := resp.Choices[0].Message.Content
	t.Logf("Response:\n%s", response)

	// Check if response looks like JSON
	trimmed := strings.TrimSpace(response)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		t.Log("✓ Response appears to be in JSON format")
	} else if strings.Contains(trimmed, "```json") {
		t.Log("✓ Response contains JSON in markdown code block")
	} else {
		t.Log("WARNING: Response may not be in JSON format")
	}

	// Check for expected answer (23 chickens, 7 cows)
	hasChickens := strings.Contains(response, "23")
	hasCows := strings.Contains(response, "7")

	if hasChickens && hasCows {
		t.Log("✓ Correct numbers (23, 7) found in response")
	} else {
		t.Log("WARNING: Expected solution (23 chickens, 7 cows) not clearly present")
	}
}

// TestReasoningContentField tests if the model exposes reasoning via the reasoning_content field
func TestReasoningContentField(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, _, err := config.Load("../../config.sft.toml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	client := NewClient(logger)
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping reasoning content test")
	}

	// Test multiple prompts to verify reasoning_content field
	testCases := []struct {
		name   string
		prompt string
	}{
		{
			name:   "Math Problem",
			prompt: "A farmer has 17 sheep. All but 9 die. How many are left?",
		},
		{
			name:   "Logic Riddle",
			prompt: "I am an odd number. Take away one letter and I become even. What number am I?",
		},
		{
			name:   "Comparison Problem",
			prompt: "Which one is bigger, 9.11 or 9.9? Think carefully.",
		},
	}

	foundReasoning := 0
	totalTests := len(testCases)
	var totalReasoningTokens int64

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			messages := []Message{
				{
					Role:    "user",
					Content: tc.prompt,
				},
			}

			mainModel := cfg.Models["main"]
			resp, err := client.ChatCompletion(ctx, mainModel, apiKey, messages)
			if err != nil {
				t.Fatalf("ChatCompletion failed: %v", err)
			}

			message := resp.Choices[0].Message
			t.Logf("Prompt: %s", tc.prompt)
			t.Logf("Answer length: %d characters", len(message.Content))

			// Check if reasoning_content field is present
			hasReasoning := message.ReasoningContent != ""

			if hasReasoning {
				foundReasoning++
				t.Log("✓ Model exposes reasoning via reasoning_content field")

				t.Logf("\n--- REASONING PROCESS ---\n%s\n", message.ReasoningContent)
				t.Logf("\n--- FINAL ANSWER ---\n%s\n", message.Content)

				t.Logf("Reasoning length: %d characters", len(message.ReasoningContent))

				// Check for reasoning tokens in usage
				if resp.Usage.ReasoningTokens > 0 {
					t.Logf("Reasoning tokens: %d", resp.Usage.ReasoningTokens)
					totalReasoningTokens += int64(resp.Usage.ReasoningTokens)
				} else if resp.Usage.CompletionTokensDetail.ReasoningTokens > 0 {
					t.Logf("Reasoning tokens (from details): %d", resp.Usage.CompletionTokensDetail.ReasoningTokens)
					totalReasoningTokens += int64(resp.Usage.CompletionTokensDetail.ReasoningTokens)
				}
			} else {
				t.Log("⚠ Model does not expose reasoning_content field")
				t.Logf("Full response:\n%s", message.Content)
			}

			// Log token usage
			if resp.Usage.TotalTokens > 0 {
				t.Logf("Token usage - Prompt: %d, Completion: %d, Total: %d",
					resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
			}
		})
	}

	// Summary
	t.Logf("\n=== REASONING CONTENT SUMMARY ===")
	t.Logf("Model: %s", cfg.Models["main"].ModelName)
	t.Logf("Tests with reasoning_content: %d/%d", foundReasoning, totalTests)
	if totalReasoningTokens > 0 {
		t.Logf("Total reasoning tokens across all tests: %d", totalReasoningTokens)
	}

	switch foundReasoning {
	case 0:
		t.Error("✗ Model does NOT expose reasoning via reasoning_content field")
		t.Log("  This may indicate the API doesn't support the reasoning_content extension.")
		t.Log("  Check if you're using a compatible endpoint (e.g., platform.moonshot.ai)")
	case totalTests:
		t.Log("✓ Model CONSISTENTLY exposes reasoning via reasoning_content field")
	default:
		t.Logf("⚠ Model SOMETIMES exposes reasoning (%d%% of responses)", (foundReasoning*100)/totalTests)
	}
}
