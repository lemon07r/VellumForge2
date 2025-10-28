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

// ChatCompletion sends a chat completion request to the specified model
func (c *Client) ChatCompletion(
	ctx context.Context,
	modelCfg config.ModelConfig,
	apiKey string,
	messages []Message,
) (*ChatCompletionResponse, error) {
	// Generate a unique model ID for rate limiting
	modelID := fmt.Sprintf("%s:%s", modelCfg.BaseURL, modelCfg.ModelName)

	// Wait for rate limiter
	if err := c.rateLimiterPool.Wait(ctx, modelID, modelCfg.RateLimitPerMinute); err != nil {
		return nil, fmt.Errorf("rate limiter wait failed: %w", err)
	}

	// Construct request
	req := ChatCompletionRequest{
		Model:       modelCfg.ModelName,
		Messages:    messages,
		Temperature: modelCfg.Temperature,
		TopP:        modelCfg.TopP,
		MaxTokens:   modelCfg.MaxOutputTokens,
		N:           1,
	}

	// Retry with exponential backoff
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff with jitter
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * c.baseRetryDelay

			// For rate limit errors, use longer delays (3^n: 6s, 18s, 54s)
			if apiErr, ok := lastErr.(*APIError); ok && apiErr.StatusCode == http.StatusTooManyRequests {
				backoff = time.Duration(math.Pow(RateLimitBackoffMultiplier, float64(attempt))) * c.baseRetryDelay
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
