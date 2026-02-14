package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/lamim/vellumforge2/internal/config"
)

// TestRawAPIResponse makes a raw API call and dumps the complete JSON response
// This helps us see exactly what the API is returning
func TestRawAPIResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, _, err := config.Load("../../config.sft.toml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping raw API test")
	}

	mainModel := cfg.Models["main"]

	// Construct request
	reqBody := map[string]interface{}{
		"model": mainModel.ModelName,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "Which one is bigger, 9.11 or 9.9? Think carefully.",
			},
		},
		"temperature": mainModel.Temperature,
		"max_tokens":  mainModel.MaxOutputTokens,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	// Make raw HTTP request
	endpoint := mainModel.BaseURL
	if endpoint[len(endpoint)-1] != '/' {
		endpoint += "/"
	}
	endpoint += "chat/completions"

	t.Logf("Making request to: %s", endpoint)
	t.Logf("Request body:\n%s\n", string(jsonData))

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	t.Logf("Response status: %d %s", resp.StatusCode, resp.Status)

	// Read raw response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	t.Logf("\n=== RAW RESPONSE BODY ===\n%s\n", string(bodyBytes))

	// Pretty print the JSON
	var prettyJSON map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &prettyJSON); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	prettyBytes, err := json.MarshalIndent(prettyJSON, "", "  ")
	if err != nil {
		t.Fatalf("Failed to format JSON: %v", err)
	}

	t.Logf("\n=== PRETTY PRINTED RESPONSE ===\n%s\n", string(prettyBytes))

	// Check for reasoning_content field
	if choices, ok := prettyJSON["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				t.Logf("\n=== MESSAGE FIELDS ===")
				for key := range message {
					t.Logf("  - %s", key)
				}

				if reasoningContent, exists := message["reasoning_content"]; exists {
					t.Logf("\n✓ reasoning_content field EXISTS!")
					if rc, ok := reasoningContent.(string); ok && rc != "" {
						t.Logf("Reasoning content length: %d characters", len(rc))
						t.Logf("First 500 chars of reasoning:\n%s\n", truncate(rc, 500))
					}
				} else {
					t.Log("\n⚠ reasoning_content field NOT FOUND in message")
				}
			}
		}
	}

	// Check for reasoning tokens in usage
	if usage, ok := prettyJSON["usage"].(map[string]interface{}); ok {
		t.Logf("\n=== USAGE FIELDS ===")
		for key, val := range usage {
			t.Logf("  - %s: %v", key, val)
		}
	}
}

// TestAPIClientWithDebug tests using our API client with debug logging
func TestAPIClientWithDebug(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, _, err := config.Load("../../config.sft.toml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Create logger with DEBUG level
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	client := NewClient(logger)
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	messages := []Message{
		{
			Role:    "user",
			Content: "Which one is bigger, 9.11 or 9.9? Think carefully.",
		},
	}

	mainModel := cfg.Models["main"]

	t.Logf("Making API call with model: %s", mainModel.ModelName)
	t.Logf("Base URL: %s", mainModel.BaseURL)
	t.Logf("Temperature: %f", mainModel.Temperature)

	resp, err := client.ChatCompletion(ctx, mainModel, apiKey, messages)
	if err != nil {
		t.Fatalf("ChatCompletion failed: %v", err)
	}

	message := resp.Choices[0].Message

	t.Logf("\n=== PARSED RESPONSE ===")
	t.Logf("Content length: %d", len(message.Content))
	t.Logf("ReasoningContent length: %d", len(message.ReasoningContent))
	t.Logf("Has reasoning_content: %v", message.ReasoningContent != "")

	if message.ReasoningContent != "" {
		t.Logf("\n--- REASONING ---\n%s\n", message.ReasoningContent)
	}

	t.Logf("\n--- ANSWER ---\n%s\n", message.Content)

	t.Logf("\n=== USAGE ===")
	t.Logf("Prompt tokens: %d", resp.Usage.PromptTokens)
	t.Logf("Completion tokens: %d", resp.Usage.CompletionTokens)
	t.Logf("Total tokens: %d", resp.Usage.TotalTokens)
	t.Logf("Reasoning tokens: %d", resp.Usage.ReasoningTokens)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// TestWithDifferentTemperatures tests if temperature affects reasoning output
func TestWithDifferentTemperatures(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg, _, err := config.Load("../../config.sft.toml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	client := NewClient(logger)

	temperatures := []float64{0.0, 1.0}
	prompt := "What is 2+2? Think step by step."

	for _, temp := range temperatures {
		t.Run(fmt.Sprintf("temp_%.1f", temp), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			mainModel := cfg.Models["main"]
			mainModel.Temperature = temp

			messages := []Message{
				{
					Role:    "user",
					Content: prompt,
				},
			}

			resp, err := client.ChatCompletion(ctx, mainModel, apiKey, messages)
			if err != nil {
				t.Fatalf("ChatCompletion failed: %v", err)
			}

			message := resp.Choices[0].Message
			t.Logf("Temperature: %.1f", temp)
			t.Logf("Has reasoning_content: %v", message.ReasoningContent != "")
			t.Logf("Reasoning length: %d", len(message.ReasoningContent))
			t.Logf("Content length: %d", len(message.Content))

			if message.ReasoningContent != "" {
				t.Logf("✓ Reasoning present at temp %.1f", temp)
			}
		})
	}
}
