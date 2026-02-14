package api

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/lamim/vellumforge2/internal/config"
)

// TestStreamingWithReasoningContent tests the streaming API with reasoning models
func TestStreamingWithReasoningContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, _, err := config.Load("../../config.sft.toml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	client := NewClient(logger)
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping streaming test")
	}

	testCases := []struct {
		name   string
		prompt string
	}{
		{
			name:   "Simple Math",
			prompt: "What is 2+2? Think step by step.",
		},
		{
			name:   "Comparison",
			prompt: "Which one is bigger, 9.11 or 9.9? Think carefully.",
		},
		{
			name:   "Logic Problem",
			prompt: "A farmer has 17 sheep. All but 9 die. How many are left?",
		},
	}

	foundReasoning := 0
	totalTests := len(testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			messages := []Message{
				{
					Role:    "user",
					Content: tc.prompt,
				},
			}

			mainModel := cfg.Models["main"]

			t.Logf("Testing STREAMING mode with prompt: %s", tc.prompt)

			resp, err := client.ChatCompletionStreaming(ctx, mainModel, apiKey, messages)
			if err != nil {
				t.Fatalf("ChatCompletionStreaming failed: %v", err)
			}

			message := resp.Choices[0].Message

			t.Logf("Response received")
			t.Logf("  Content length: %d characters", len(message.Content))
			t.Logf("  Reasoning length: %d characters", len(message.ReasoningContent))

			hasReasoning := message.ReasoningContent != ""

			if hasReasoning {
				foundReasoning++
				t.Log("✓ Reasoning content found via STREAMING!")

				t.Logf("\n--- REASONING PROCESS ---\n%s\n", message.ReasoningContent)
				t.Logf("\n--- FINAL ANSWER ---\n%s\n", message.Content)
			} else {
				t.Log("⚠ No reasoning content (model may have responded without reasoning)")
				t.Logf("Full response:\n%s", message.Content)
			}

			// Check finish reason
			if resp.Choices[0].FinishReason == "length" {
				t.Log("WARNING: Response truncated (finish_reason: length)")
			}
		})
	}

	// Summary
	t.Logf("\n=== STREAMING REASONING SUMMARY ===")
	t.Logf("Model: %s", cfg.Models["main"].ModelName)
	t.Logf("Tests with reasoning_content: %d/%d", foundReasoning, totalTests)

	switch foundReasoning {
	case 0:
		t.Error("✗ No reasoning content found in any streaming responses")
		t.Log("  This suggests the model or endpoint may not support reasoning")
	case totalTests:
		t.Log("✓ ALL responses included reasoning content via streaming!")
	default:
		t.Logf("⚠ SOME responses included reasoning (%d%% success rate)", (foundReasoning*100)/totalTests)
		t.Log("  Reasoning models may skip reasoning for simple queries")
	}
}

// TestStreamingVsNonStreaming compares streaming vs non-streaming responses
func TestStreamingVsNonStreaming(t *testing.T) {
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
		t.Skip("OPENAI_API_KEY not set")
	}

	prompt := "Which is bigger, 9.11 or 9.9? Think step by step."
	messages := []Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	mainModel := cfg.Models["main"]

	// Test non-streaming
	t.Log("Testing NON-STREAMING mode...")
	ctx1, cancel1 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel1()

	nonStreamResp, err := client.ChatCompletion(ctx1, mainModel, apiKey, messages)
	if err != nil {
		t.Fatalf("Non-streaming request failed: %v", err)
	}

	nonStreamMsg := nonStreamResp.Choices[0].Message
	t.Logf("Non-streaming result:")
	t.Logf("  Content length: %d", len(nonStreamMsg.Content))
	t.Logf("  Reasoning length: %d", len(nonStreamMsg.ReasoningContent))
	t.Logf("  Has reasoning: %v", nonStreamMsg.ReasoningContent != "")

	// Test streaming
	t.Log("\nTesting STREAMING mode...")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel2()

	streamResp, err := client.ChatCompletionStreaming(ctx2, mainModel, apiKey, messages)
	if err != nil {
		t.Fatalf("Streaming request failed: %v", err)
	}

	streamMsg := streamResp.Choices[0].Message
	t.Logf("Streaming result:")
	t.Logf("  Content length: %d", len(streamMsg.Content))
	t.Logf("  Reasoning length: %d", len(streamMsg.ReasoningContent))
	t.Logf("  Has reasoning: %v", streamMsg.ReasoningContent != "")

	// Compare
	t.Log("\n=== COMPARISON ===")
	if nonStreamMsg.ReasoningContent == "" && streamMsg.ReasoningContent != "" {
		t.Log("✓ CONFIRMED: Reasoning content ONLY available via streaming!")
		t.Logf("   Streaming reasoning: %d characters", len(streamMsg.ReasoningContent))
	} else if nonStreamMsg.ReasoningContent != "" {
		t.Log("✓ Reasoning available in both modes")
	} else {
		t.Log("⚠ No reasoning content in either mode")
	}
}
