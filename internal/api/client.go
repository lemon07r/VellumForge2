package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/lamim/vellumforge2/internal/config"
)

const (
	// DefaultHTTPTimeout is the default timeout for HTTP requests
	DefaultHTTPTimeout = 120 * time.Second
	// DefaultMaxRetries is the default maximum number of retry attempts
	DefaultMaxRetries = 3
	// DefaultBaseRetryDelay is the base delay for exponential backoff
	DefaultBaseRetryDelay = 2 * time.Second
	// RateLimitBackoffMultiplier is the multiplier for rate limit backoff (3^n)
	RateLimitBackoffMultiplier = 3
	// DefaultMaxBackoffDuration is the default maximum backoff duration
	DefaultMaxBackoffDuration = 2 * time.Minute
)

// Client handles HTTP requests to OpenAI-compatible API endpoints
type Client struct {
	httpClient      *http.Client
	rateLimiterPool *RateLimiterPool
	logger          *slog.Logger
	maxRetries      int
	baseRetryDelay  time.Duration
}

// NewClient creates a new API client
func NewClient(logger *slog.Logger) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: DefaultHTTPTimeout,
		},
		rateLimiterPool: NewRateLimiterPool(),
		logger:          logger,
		maxRetries:      DefaultMaxRetries,
		baseRetryDelay:  DefaultBaseRetryDelay,
	}
}

// SetMaxRetries sets the maximum number of retry attempts
func (c *Client) SetMaxRetries(maxRetries int) {
	c.maxRetries = maxRetries
}

// ChatCompletion sends a chat completion request to the specified model
func (c *Client) ChatCompletion(
	ctx context.Context,
	modelCfg config.ModelConfig,
	apiKey string,
	messages []Message,
) (*ChatCompletionResponse, error) {
	requestStart := time.Now()
	
	// Generate a unique model ID for rate limiting
	modelID := fmt.Sprintf("%s:%s", modelCfg.BaseURL, modelCfg.ModelName)

	// Wait for rate limiter
	rateLimitStart := time.Now()
	if err := c.rateLimiterPool.Wait(ctx, modelID, modelCfg.RateLimitPerMinute); err != nil {
		return nil, fmt.Errorf("rate limiter wait failed: %w", err)
	}
	rateLimitWait := time.Since(rateLimitStart)

	// Construct request
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

	// Retry with exponential backoff
	// Use model-specific maxRetries if configured (default is 3 from loader)
	// Set to -1 for unlimited retries
	var lastErr error
	maxAttempts := modelCfg.MaxRetries
	if maxAttempts == 0 {
		maxAttempts = c.maxRetries // Fallback to client default
	}
	
	apiCallStart := time.Now()
	for attempt := 0; maxAttempts < 0 || attempt <= maxAttempts; attempt++ {
		if attempt > 0 {
			// Calculate backoff with jitter
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * c.baseRetryDelay

			// For rate limit errors, use longer delays (3^n: 6s, 18s, 54s)
			if apiErr, ok := lastErr.(*APIError); ok && apiErr.StatusCode == http.StatusTooManyRequests {
				backoff = time.Duration(math.Pow(RateLimitBackoffMultiplier, float64(attempt))) * c.baseRetryDelay
			}

			// Apply configurable backoff cap
			maxBackoff := DefaultMaxBackoffDuration
			if modelCfg.MaxBackoffSeconds > 0 {
				maxBackoff = time.Duration(modelCfg.MaxBackoffSeconds) * time.Second
			}
			if backoff > maxBackoff {
				backoff = maxBackoff
			}

			jitter := time.Duration(float64(backoff) * 0.1 * (2*float64(time.Now().UnixNano()%100)/100 - 1))
			sleepDuration := backoff + jitter

			c.logger.Warn("Retrying API request",
				"attempt", attempt,
				"max_retries", c.maxRetries,
				"backoff", sleepDuration,
				"model", modelCfg.ModelName,
				"is_rate_limit", lastErr != nil && c.isRateLimitError(lastErr))

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleepDuration):
			}
		}

		resp, err := c.doRequest(ctx, modelCfg.BaseURL, apiKey, req)
		if err == nil {
			apiCallDuration := time.Since(apiCallStart)
			totalDuration := time.Since(requestStart)
			
			// Log performance metrics
			c.logger.Debug("API request completed",
				"model", modelCfg.ModelName,
				"rate_limit_wait_ms", rateLimitWait.Milliseconds(),
				"api_duration_ms", apiCallDuration.Milliseconds(),
				"total_ms", totalDuration.Milliseconds())
			
			// Check finish_reason for truncation
			if len(resp.Choices) > 0 && resp.Choices[0].FinishReason == "length" {
				c.logger.Warn("Response truncated due to max_tokens limit",
					"model", modelCfg.ModelName,
					"max_tokens", modelCfg.MaxOutputTokens,
					"finish_reason", resp.Choices[0].FinishReason)
			}
			return resp, nil
		}

		lastErr = err

		// Check if error is retryable
		if !c.isRetryable(err) {
			return nil, err
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// ChatCompletionStructured sends a chat completion request optimized for structured JSON output
// Uses structure_temperature if set, otherwise falls back to regular temperature
// Automatically enables JSON mode if configured
func (c *Client) ChatCompletionStructured(
	ctx context.Context,
	modelCfg config.ModelConfig,
	apiKey string,
	messages []Message,
) (*ChatCompletionResponse, error) {
	// Use structure_temperature if set, otherwise use regular temperature
	tempCfg := modelCfg
	if modelCfg.StructureTemperature > 0 {
		tempCfg.Temperature = modelCfg.StructureTemperature
		c.logger.Debug("Using structure_temperature for JSON generation",
			"structure_temp", modelCfg.StructureTemperature,
			"regular_temp", modelCfg.Temperature)
	}

	// Call regular ChatCompletion with modified config
	return c.ChatCompletion(ctx, tempCfg, apiKey, messages)
}

func (c *Client) doRequest(
	ctx context.Context,
	baseURL string,
	apiKey string,
	req ChatCompletionRequest,
) (*ChatCompletionResponse, error) {
	// Marshal request body
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	endpoint := baseURL
	if endpoint[len(endpoint)-1] != '/' {
		endpoint += "/"
	}
	endpoint += "chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		c.logger.Debug("API request", "endpoint", endpoint, "has_key", true, "key_length", len(apiKey))
	} else {
		c.logger.Warn("API request without key", "endpoint", endpoint)
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
	defer func() {
		if err := httpResp.Body.Close(); err != nil {
			c.logger.Warn("Failed to close response body", "error", err)
		}
	}()

	// Read response body
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if httpResp.StatusCode != http.StatusOK {
		isRetryable := c.isStatusCodeRetryable(httpResp.StatusCode)

		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, &APIError{
				Message:    errResp.Error.Message,
				StatusCode: httpResp.StatusCode,
				Type:       errResp.Error.Type,
				Code:       errResp.Error.Code,
				Retryable:  isRetryable,
			}
		}

		return nil, &APIError{
			Message:    fmt.Sprintf("API request failed with status %d: %s", httpResp.StatusCode, string(respBody)),
			StatusCode: httpResp.StatusCode,
			Retryable:  isRetryable,
		}
	}

	// Parse response
	var resp ChatCompletionResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned in response")
	}

	return &resp, nil
}

func (c *Client) isRetryable(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.Retryable
	}
	return false
}

func (c *Client) isRateLimitError(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == http.StatusTooManyRequests
	}
	return false
}

func (c *Client) isStatusCodeRetryable(statusCode int) bool {
	// Retry on rate limits and server errors
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusInternalServerError ||
		statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout
}

// APIError represents an error returned by the API
type APIError struct {
	Message    string
	StatusCode int
	Type       string
	Code       string
	Retryable  bool
}

func (e *APIError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("API error (status %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("API error: %s", e.Message)
}
