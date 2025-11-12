package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/lamim/vellumforge2/internal/config"
)

// StreamDelta represents the delta content in a streaming response chunk
type StreamDelta struct {
	Role             string `json:"role,omitempty"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"` // For reasoning models
}

// StreamChoice represents a choice in a streaming response chunk
type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        StreamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason,omitempty"`
}

// StreamResponse represents a single chunk in the streaming response
type StreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
}

// ChatCompletionStreaming sends a chat completion request with streaming enabled
// This is required for reasoning models to expose the reasoning_content field
func (c *Client) ChatCompletionStreaming(
	ctx context.Context,
	modelCfg config.ModelConfig,
	apiKey string,
	messages []Message,
) (*ChatCompletionResponse, error) {
	requestStart := time.Now()

	// Apply per-model HTTP timeout
	// HTTPTimeoutSeconds should always be set (config loader defaults to 120s)
	// For long-form generation, increase this value in config
	var cancel context.CancelFunc
	timeout := time.Duration(modelCfg.HTTPTimeoutSeconds) * time.Second
	if timeout == 0 {
		// Fallback to default if somehow not set
		timeout = DefaultHTTPTimeout
		c.logger.Warn("Model has no timeout configured, using default",
			"model", modelCfg.ModelName,
			"timeout", timeout)
	}
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	// Generate a unique model ID for rate limiting
	modelID := fmt.Sprintf("%s:%s", modelCfg.BaseURL, modelCfg.ModelName)

	// Get provider name and check for provider-level rate limit
	providerName := config.GetProviderName(modelCfg.BaseURL)
	providerRPM := 0
	if c.providerRateLimits != nil {
		if rpm, ok := c.providerRateLimits[providerName]; ok {
			providerRPM = rpm
		}
	}

	// Wait for rate limiter
	rateLimitStart := time.Now()
	if err := c.rateLimiterPool.Wait(ctx, modelID, modelCfg.RateLimitPerMinute, providerName, providerRPM, c.providerBurstPercent); err != nil {
		return nil, fmt.Errorf("rate limiter wait failed: %w", err)
	}
	rateLimitWait := time.Since(rateLimitStart)

	// Construct request with streaming enabled
	req := ChatCompletionRequest{
		Model:       modelCfg.ModelName,
		Messages:    messages,
		Temperature: modelCfg.Temperature,
		TopP:        modelCfg.TopP,
		MaxTokens:   modelCfg.MaxOutputTokens,
		N:           1,
	}

	// Enable JSON mode if configured
	if modelCfg.UseJSONMode {
		req.ResponseFormat = &ResponseFormat{Type: "json_object"}
	}

	// Add stream parameter (not part of ChatCompletionRequest struct)
	reqMap := make(map[string]interface{})
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	if err := json.Unmarshal(reqBytes, &reqMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}
	reqMap["stream"] = true

	// Retry with exponential backoff
	var lastErr error
	maxAttempts := modelCfg.MaxRetries
	if maxAttempts == 0 {
		maxAttempts = c.maxRetries
	}

	apiCallStart := time.Now()
	for attempt := 0; maxAttempts < 0 || attempt <= maxAttempts; attempt++ {
		if attempt > 0 {
			// Calculate backoff
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * c.baseRetryDelay
			if apiErr, ok := lastErr.(*APIError); ok && apiErr.StatusCode == http.StatusTooManyRequests {
				backoff = time.Duration(math.Pow(RateLimitBackoffMultiplier, float64(attempt))) * c.baseRetryDelay
			}

			maxBackoff := DefaultMaxBackoffDuration
			if modelCfg.MaxBackoffSeconds > 0 {
				maxBackoff = time.Duration(modelCfg.MaxBackoffSeconds) * time.Second
			}
			if backoff > maxBackoff {
				backoff = maxBackoff
			}

			jitter := time.Duration(float64(backoff) * 0.1 * (2*float64(time.Now().UnixNano()%100)/100 - 1))
			sleepDuration := backoff + jitter

			c.logger.Warn("Retrying streaming API request",
				"attempt", attempt,
				"max_retries", maxAttempts,
				"backoff", sleepDuration,
				"model", modelCfg.ModelName)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleepDuration):
			}
		}

		resp, err := c.doStreamingRequest(ctx, modelCfg.BaseURL, apiKey, reqMap)
		if err == nil {
			apiCallDuration := time.Since(apiCallStart)
			totalDuration := time.Since(requestStart)

			c.logger.Debug("Streaming API request completed",
				"model", modelCfg.ModelName,
				"rate_limit_wait_ms", rateLimitWait.Milliseconds(),
				"api_duration_ms", apiCallDuration.Milliseconds(),
				"total_ms", totalDuration.Milliseconds())

			return resp, nil
		}

		lastErr = err

		if !c.isRetryable(err) {
			return nil, err
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (c *Client) doStreamingRequest(
	ctx context.Context,
	baseURL string,
	apiKey string,
	reqMap map[string]interface{},
) (*ChatCompletionResponse, error) {
	// Encode request
	buf := getBuffer()
	defer putBuffer(buf)

	if err := json.NewEncoder(buf).Encode(reqMap); err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	endpoint := baseURL
	if endpoint[len(endpoint)-1] != '/' {
		endpoint += "/"
	}
	endpoint += "chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	// Send request
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &APIError{
			Message:    fmt.Sprintf("request failed: %v", err),
			StatusCode: 0,
			Retryable:  true,
		}
	}
	defer httpResp.Body.Close()

	// Check status code
	if httpResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(httpResp.Body)
		var errResp ErrorResponse
		if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, &APIError{
				Message:    errResp.Error.Message,
				StatusCode: httpResp.StatusCode,
				Type:       errResp.Error.Type,
				Code:       errResp.Error.Code,
				Retryable:  c.isStatusCodeRetryable(httpResp.StatusCode),
			}
		}

		return nil, &APIError{
			Message:    fmt.Sprintf("API request failed with status %d: %s", httpResp.StatusCode, string(bodyBytes)),
			StatusCode: httpResp.StatusCode,
			Retryable:  c.isStatusCodeRetryable(httpResp.StatusCode),
		}
	}

	// Read streaming response
	var responseContent strings.Builder
	var reasoningContent strings.Builder
	var responseID string
	var responseModel string
	var responseCreated int64
	var finishReason string

	scanner := bufio.NewScanner(httpResp.Body)
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
				break
			}

			// Parse JSON chunk
			var chunk StreamResponse
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				c.logger.Warn("Failed to parse stream chunk", "error", err, "data", data)
				continue
			}

			// Store metadata from first chunk
			if responseID == "" {
				responseID = chunk.ID
				responseModel = chunk.Model
				responseCreated = chunk.Created
			}

			// Extract content from delta
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta

				// Append regular content
				if delta.Content != "" {
					responseContent.WriteString(delta.Content)
				}

				// Append reasoning content (for reasoning models)
				if delta.ReasoningContent != "" {
					reasoningContent.WriteString(delta.ReasoningContent)
				}

				// Check finish reason
				if chunk.Choices[0].FinishReason != nil && *chunk.Choices[0].FinishReason != "" {
					finishReason = *chunk.Choices[0].FinishReason
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("stream reading error: %w", err)
	}

	// Construct final response
	finalMessage := Message{
		Role:             "assistant",
		Content:          responseContent.String(),
		ReasoningContent: reasoningContent.String(),
	}

	resp := &ChatCompletionResponse{
		ID:      responseID,
		Object:  "chat.completion",
		Created: responseCreated,
		Model:   responseModel,
		Choices: []Choice{
			{
				Index:        0,
				Message:      finalMessage,
				FinishReason: finishReason,
			},
		},
		Usage: Usage{
			// Note: Token counts not available in streaming mode
			// Would need to estimate or use tiktoken
			CompletionTokens: 0,
			PromptTokens:     0,
			TotalTokens:      0,
		},
	}

	// Log reasoning detection
	if reasoningContent.Len() > 0 {
		c.logger.Debug("Reasoning content detected",
			"model", responseModel,
			"reasoning_length", reasoningContent.Len(),
			"content_length", responseContent.Len())
	}

	return resp, nil
}
