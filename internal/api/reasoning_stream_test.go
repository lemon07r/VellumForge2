package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lamim/vellumforge2/internal/config"
)

// TestStreamingForReasoning tests if streaming mode exposes reasoning_content
func TestStreamingForReasoning(t *testing.T) {
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

	mainModel := cfg.Models["main"]

	// Construct request with streaming enabled
	reqBody := map[string]interface{}{
		"model": mainModel.ModelName,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "What is 2+2? Think step by step.",
			},
		},
		"temperature": 1.0,
		"max_tokens":  4096,
		"stream":      true, // Enable streaming
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	endpoint := mainModel.BaseURL
	if endpoint[len(endpoint)-1] != '/' {
		endpoint += "/"
	}
	endpoint += "chat/completions"

	t.Logf("Testing STREAMING mode")
	t.Logf("Endpoint: %s", endpoint)

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	t.Logf("Response status: %d", resp.StatusCode)

	// Read streaming response
	scanner := bufio.NewScanner(resp.Body)
	chunkCount := 0
	hasReasoningContent := false
	var reasoningContent strings.Builder
	var finalContent strings.Builder

	t.Log("\n=== STREAMING CHUNKS ===")

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines
		if len(strings.TrimSpace(line)) == 0 {
			continue
		}

		// SSE format: "data: {...}"
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Check for end marker
			if data == "[DONE]" {
				t.Log("Stream ended with [DONE]")
				break
			}

			chunkCount++

			// Parse JSON
			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				t.Logf("Warning: Failed to parse chunk %d: %v", chunkCount, err)
				t.Logf("Raw chunk: %s", data)
				continue
			}

			// Show first few chunks in detail
			if chunkCount <= 3 {
				prettyChunk, _ := json.MarshalIndent(chunk, "", "  ")
				t.Logf("\n--- Chunk %d ---\n%s", chunkCount, string(prettyChunk))
			}

			// Extract delta content
			if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						// Check for reasoning_content in delta
						if rc, exists := delta["reasoning_content"]; exists {
							hasReasoningContent = true
							if rcStr, ok := rc.(string); ok {
								reasoningContent.WriteString(rcStr)
								t.Logf("✓ Found reasoning_content in chunk %d (length: %d)", chunkCount, len(rcStr))
							}
						}

						// Check for regular content
						if content, exists := delta["content"]; exists {
							if contentStr, ok := content.(string); ok {
								finalContent.WriteString(contentStr)
							}
						}
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("Scanner error: %v", err)
	}

	t.Logf("\n=== STREAM SUMMARY ===")
	t.Logf("Total chunks received: %d", chunkCount)
	t.Logf("Has reasoning_content: %v", hasReasoningContent)
	t.Logf("Reasoning length: %d characters", reasoningContent.Len())
	t.Logf("Content length: %d characters", finalContent.Len())

	if hasReasoningContent {
		t.Logf("\n--- REASONING ---\n%s\n", reasoningContent.String())
	}

	if finalContent.Len() > 0 {
		t.Logf("\n--- FINAL ANSWER ---\n%s\n", finalContent.String())
	}

	if !hasReasoningContent {
		t.Log("\n⚠ No reasoning_content found in streaming mode either")
	}
}

// TestWithExplicitReasoningParameter tests if there's a parameter to enable reasoning
func TestWithExplicitReasoningParameter(t *testing.T) {
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

	mainModel := cfg.Models["main"]

	// Try different parameter combinations that might enable reasoning
	testCases := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name: "with_stream_options",
			params: map[string]interface{}{
				"model": mainModel.ModelName,
				"messages": []map[string]string{
					{"role": "user", "content": "What is 2+2?"},
				},
				"temperature": 1.0,
				"stream":      false,
				"stream_options": map[string]bool{
					"include_usage": true,
				},
			},
		},
		{
			name: "with_reasoning_effort",
			params: map[string]interface{}{
				"model": mainModel.ModelName,
				"messages": []map[string]string{
					{"role": "user", "content": "What is 2+2?"},
				},
				"temperature":      1.0,
				"reasoning_effort": "high",
			},
		},
		{
			name: "with_thinking_mode",
			params: map[string]interface{}{
				"model": mainModel.ModelName,
				"messages": []map[string]string{
					{"role": "user", "content": "What is 2+2?"},
				},
				"temperature": 1.0,
				"thinking":    true,
			},
		},
		{
			name: "with_show_reasoning",
			params: map[string]interface{}{
				"model": mainModel.ModelName,
				"messages": []map[string]string{
					{"role": "user", "content": "What is 2+2?"},
				},
				"temperature":    1.0,
				"show_reasoning": true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tc.params)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			endpoint := mainModel.BaseURL
			if endpoint[len(endpoint)-1] != '/' {
				endpoint += "/"
			}
			endpoint += "chat/completions"

			t.Logf("Testing with parameters: %s", string(jsonData))

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

			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response: %v", err)
			}

			var respData map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &respData); err != nil {
				t.Logf("Response: %s", string(bodyBytes))
				t.Fatalf("Failed to parse response: %v", err)
			}

			// Check for reasoning_content
			if choices, ok := respData["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if message, ok := choice["message"].(map[string]interface{}); ok {
						if _, exists := message["reasoning_content"]; exists {
							t.Logf("✓ reasoning_content FOUND with %s!", tc.name)
							prettyResp, _ := json.MarshalIndent(respData, "", "  ")
							t.Logf("Response:\n%s", string(prettyResp))
						} else {
							t.Logf("⚠ No reasoning_content with %s", tc.name)
						}
					}
				}
			}
		})
	}
}
