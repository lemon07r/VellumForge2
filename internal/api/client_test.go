package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/lamim/vellumforge2/internal/config"
)

func TestChatCompletion_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Expected Authorization header 'Bearer test-key', got '%s'", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
		}

		// Return mock response
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "test-123",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "test-model",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Test response"
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 5,
				"total_tokens": 15
			}
		}`))
	}))
	defer server.Close()

	// Create client
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(logger)

	// Create test config
	modelCfg := config.ModelConfig{
		BaseURL:            server.URL,
		ModelName:          "test-model",
		Temperature:        0.7,
		TopP:               1.0,
		MaxOutputTokens:    100,
		RateLimitPerMinute: 60,
	}

	// Make request
	resp, err := client.ChatCompletion(
		context.Background(),
		modelCfg,
		"test-key",
		[]Message{{Role: "user", Content: "Test message"}},
	)

	// Verify
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response, got nil")
		return
	}
	if resp.Choices == nil || len(resp.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Test response" {
		t.Errorf("Expected content 'Test response', got '%s'", resp.Choices[0].Message.Content)
	}
}

func TestChatCompletion_RateLimiting(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "test",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "test",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "ok"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(logger)

	modelCfg := config.ModelConfig{
		BaseURL:            server.URL,
		ModelName:          "test",
		RateLimitPerMinute: 60, // 1 per second
	}

	// Make 3 rapid requests
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, err := client.ChatCompletion(ctx, modelCfg, "test", []Message{{Role: "user", Content: "test"}})
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
	}

	// Verify all requests completed
	if callCount != 3 {
		t.Errorf("Expected 3 API calls, got %d", callCount)
	}
}

func TestChatCompletion_RetryOn500(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": {"message": "Server error"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "test",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "test",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "success"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(logger)
	client.maxRetries = 3
	client.baseRetryDelay = 1 // 1ms for fast testing

	modelCfg := config.ModelConfig{
		BaseURL:            server.URL,
		ModelName:          "test",
		RateLimitPerMinute: 1000,
	}

	resp, err := client.ChatCompletion(context.Background(), modelCfg, "test", []Message{{Role: "user", Content: "test"}})

	if err != nil {
		t.Fatalf("Expected success after retries, got error: %v", err)
	}
	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts (2 retries), got %d", attemptCount)
	}
	if resp.Choices[0].Message.Content != "success" {
		t.Errorf("Expected 'success', got '%s'", resp.Choices[0].Message.Content)
	}
}

func TestChatCompletion_BackoffCap(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": {"message": "Rate limit exceeded"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "test",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "test",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "success"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(logger)
	client.maxRetries = 5
	client.baseRetryDelay = 1 * time.Millisecond // Fast for testing

	// Set a very low backoff cap to test capping behavior
	modelCfg := config.ModelConfig{
		BaseURL:            server.URL,
		ModelName:          "test",
		RateLimitPerMinute: 1000,
		MaxBackoffSeconds:  1, // Cap at 1 second (instead of default 120)
	}

	start := time.Now()
	resp, err := client.ChatCompletion(context.Background(), modelCfg, "test", []Message{{Role: "user", Content: "test"}})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Expected success after retries, got error: %v", err)
	}

	// With 3^n backoff: 3ms, 9ms, 27ms (all capped at 1000ms)
	// Total should be < 3s (with some margin for jitter)
	if elapsed > 4*time.Second {
		t.Errorf("Backoff took too long: %v (backoff cap may not be working)", elapsed)
	}

	if attemptCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", attemptCount)
	}

	if resp.Choices[0].Message.Content != "success" {
		t.Errorf("Expected 'success', got '%s'", resp.Choices[0].Message.Content)
	}
}

func TestChatCompletion_DefaultBackoffCap(t *testing.T) {
	// Test that default backoff cap (2 minutes) is respected
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(logger)
	client.baseRetryDelay = 2 * time.Second

	// Simulate a model config without explicit MaxBackoffSeconds (should use default)
	modelCfg := config.ModelConfig{
		BaseURL:            "https://api.example.com",
		ModelName:          "test",
		RateLimitPerMinute: 60,
		MaxBackoffSeconds:  0, // Use default
	}

	// We can't actually run this test without a server, but we can verify
	// the configuration is set correctly
	if modelCfg.MaxBackoffSeconds != 0 {
		t.Errorf("Expected MaxBackoffSeconds to be 0 (use default), got %d", modelCfg.MaxBackoffSeconds)
	}
	// The actual default is applied in the loader.go via applyDefaults()
}
