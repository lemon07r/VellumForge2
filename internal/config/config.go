package config

import (
	"fmt"
	"os"
	"strings"
)

// Config represents the complete application configuration
type Config struct {
	Generation           GenerationConfig       `toml:"generation"`
	Models               map[string]ModelConfig `toml:"models"`
	PromptTemplates      PromptTemplates        `toml:"prompt_templates"`
	HuggingFace          HuggingFaceConfig      `toml:"huggingface"`
	ProviderRateLimits   map[string]int         `toml:"provider_rate_limits"`   // Global rate limits per provider (requests per minute)
	ProviderBurstPercent int                    `toml:"provider_burst_percent"` // Burst capacity as percentage (1-50, default: 15)
}

// GenerationConfig holds generation-specific settings
type GenerationConfig struct {
	MainTopic               string  `toml:"main_topic"`
	NumSubtopics            int     `toml:"num_subtopics"`
	SubtopicChunkSize       int     `toml:"subtopic_chunk_size"` // Request subtopics in chunks (0=all at once, default: 30)
	NumPromptsPerSubtopic   int     `toml:"num_prompts_per_subtopic"`
	Concurrency             int     `toml:"concurrency"`
	OverGenerationBuffer    float64 `toml:"over_generation_buffer"`    // Buffer percentage (0.0-1.0, default 0.15)
	MaxExclusionListSize    int     `toml:"max_exclusion_list_size"`   // Max items in exclusion list (default 50)
	DisableValidationLimits bool    `toml:"disable_validation_limits"` // Disable upper bound validation (use with caution)
	EnableCheckpointing     bool    `toml:"enable_checkpointing"`      // Enable checkpoint/resume support
	CheckpointInterval      int     `toml:"checkpoint_interval"`       // Save checkpoint every N completed jobs (default: 10)
	ResumeFromSession       string  `toml:"resume_from_session"`       // Session directory to resume from (e.g., "session_2025-10-27T12-34-56")
}

// ModelConfig represents configuration for a single model endpoint
type ModelConfig struct {
	BaseURL              string  `toml:"base_url"`
	ModelName            string  `toml:"model_name"`
	Temperature          float64 `toml:"temperature"`
	StructureTemperature float64 `toml:"structure_temperature"`      // Temperature for JSON generation (optional, defaults to temperature)
	TopP                 float64 `toml:"top_p"`
	TopK                 int     `toml:"top_k"`
	MinP                 float64 `toml:"min_p"`
	MaxOutputTokens      int     `toml:"max_output_tokens"`
	ContextSize          int     `toml:"context_size"`
	RateLimitPerMinute   int     `toml:"rate_limit_per_minute"`
	MaxBackoffSeconds    int     `toml:"max_backoff_seconds"`        // Optional: max backoff duration (default 120)
	MaxRetries           int     `toml:"max_retries"`                // Optional: max retry attempts (default 3, 0 = unlimited)
	JudgeTimeoutSeconds  int     `toml:"judge_timeout_seconds,omitempty"` // Timeout for judge API calls (default: 100s)
	UseJSONMode          bool    `toml:"use_json_mode"`              // Enable structured JSON output mode (optional)
	Enabled              bool    `toml:"enabled"`                    // Only used for judge model
}

// PromptTemplates holds all customizable prompt templates
type PromptTemplates struct {
	SubtopicGeneration string `toml:"subtopic_generation"`
	PromptGeneration   string `toml:"prompt_generation"`
	ChosenGeneration   string `toml:"chosen_generation"`
	RejectedGeneration string `toml:"rejected_generation"`
	JudgeRubric        string `toml:"judge_rubric"`
}

// HuggingFaceConfig holds Hugging Face Hub settings
type HuggingFaceConfig struct {
	RepoID string `toml:"repo_id"`
}

// Secrets holds sensitive credentials loaded from environment variables
type Secrets struct {
	APIKeys          map[string]string
	HuggingFaceToken string
}

const (
	// MaxConcurrency is the maximum allowed concurrency
	MaxConcurrency = 1024
	// MaxNumSubtopics is the maximum allowed subtopics
	MaxNumSubtopics = 10000
	// MaxNumPromptsPerSubtopic is the maximum prompts per subtopic
	MaxNumPromptsPerSubtopic = 10000
)

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Set default provider burst percent if not specified
	if c.ProviderBurstPercent == 0 {
		c.ProviderBurstPercent = 15 // Default: 15% burst
	}
	// Validate provider burst percent range
	if c.ProviderBurstPercent < 1 || c.ProviderBurstPercent > 50 {
		return fmt.Errorf("provider_burst_percent must be between 1 and 50 (got %d)", c.ProviderBurstPercent)
	}

	// Validate generation config
	if c.Generation.MainTopic == "" {
		return fmt.Errorf("generation.main_topic is required")
	}
	if c.Generation.NumSubtopics < 1 {
		return fmt.Errorf("generation.num_subtopics must be at least 1")
	}
	// Skip upper bound validation if disabled
	if !c.Generation.DisableValidationLimits {
		if c.Generation.NumSubtopics > MaxNumSubtopics {
			return fmt.Errorf("generation.num_subtopics must not exceed %d (got %d)", MaxNumSubtopics, c.Generation.NumSubtopics)
		}
	}
	if c.Generation.NumPromptsPerSubtopic < 1 {
		return fmt.Errorf("generation.num_prompts_per_subtopic must be at least 1")
	}
	// Skip upper bound validation if disabled
	if !c.Generation.DisableValidationLimits {
		if c.Generation.NumPromptsPerSubtopic > MaxNumPromptsPerSubtopic {
			return fmt.Errorf("generation.num_prompts_per_subtopic must not exceed %d (got %d)", MaxNumPromptsPerSubtopic, c.Generation.NumPromptsPerSubtopic)
		}
	}
	if c.Generation.Concurrency < 1 {
		return fmt.Errorf("generation.concurrency must be at least 1")
	}
	// Skip upper bound validation if disabled
	if !c.Generation.DisableValidationLimits {
		if c.Generation.Concurrency > MaxConcurrency {
			return fmt.Errorf("generation.concurrency must not exceed %d (got %d)", MaxConcurrency, c.Generation.Concurrency)
		}
	}
	if c.Generation.OverGenerationBuffer < 0 || c.Generation.OverGenerationBuffer > 1.0 {
		return fmt.Errorf("generation.over_generation_buffer must be between 0.0 and 1.0 (got %.2f)", c.Generation.OverGenerationBuffer)
	}
	if c.Generation.CheckpointInterval < 1 {
		// Set default if not specified
		c.Generation.CheckpointInterval = 10
	}

	// Validate main model exists
	mainModel, ok := c.Models["main"]
	if !ok {
		return fmt.Errorf("models.main is required")
	}
	if err := validateModelConfig("main", mainModel); err != nil {
		return err
	}

	// Validate rejected model exists
	rejectedModel, ok := c.Models["rejected"]
	if !ok {
		return fmt.Errorf("models.rejected is required")
	}
	if err := validateModelConfig("rejected", rejectedModel); err != nil {
		return err
	}

	// Validate judge model if enabled
	if judgeModel, ok := c.Models["judge"]; ok && judgeModel.Enabled {
		if err := validateModelConfig("judge", judgeModel); err != nil {
			return err
		}
		if c.PromptTemplates.JudgeRubric == "" {
			return fmt.Errorf("prompt_templates.judge_rubric is required when judge is enabled")
		}
	}

	// Validate prompt templates
	if c.PromptTemplates.SubtopicGeneration == "" {
		return fmt.Errorf("prompt_templates.subtopic_generation is required")
	}
	if c.PromptTemplates.PromptGeneration == "" {
		return fmt.Errorf("prompt_templates.prompt_generation is required")
	}

	return nil
}

func validateModelConfig(name string, mc ModelConfig) error {
	if mc.BaseURL == "" {
		return fmt.Errorf("models.%s.base_url is required", name)
	}
	if mc.ModelName == "" {
		return fmt.Errorf("models.%s.model_name is required", name)
	}
	if mc.Temperature < 0 || mc.Temperature > 2 {
		return fmt.Errorf("models.%s.temperature must be between 0 and 2", name)
	}
	if mc.TopP < 0 || mc.TopP > 1 {
		return fmt.Errorf("models.%s.top_p must be between 0 and 1", name)
	}
	if mc.MaxOutputTokens < 1 {
		return fmt.Errorf("models.%s.max_output_tokens must be at least 1", name)
	}
	if mc.ContextSize < 1 {
		return fmt.Errorf("models.%s.context_size must be at least 1", name)
	}
	if mc.RateLimitPerMinute < 1 {
		return fmt.Errorf("models.%s.rate_limit_per_minute must be at least 1", name)
	}
	if mc.MaxOutputTokens > mc.ContextSize {
		return fmt.Errorf("models.%s.max_output_tokens (%d) must not exceed context_size (%d)", name, mc.MaxOutputTokens, mc.ContextSize)
	}
	return nil
}

// LoadSecrets loads sensitive credentials from environment variables
func LoadSecrets() (*Secrets, error) {
	secrets := &Secrets{
		APIKeys: make(map[string]string),
	}

	// Load generic API key (provider-agnostic)
	if key := os.Getenv("API_KEY"); key != "" {
		secrets.APIKeys["generic"] = key
	}

	// Load provider-specific API keys (optional, override generic)
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		secrets.APIKeys["openai"] = key
	}
	if key := os.Getenv("NVIDIA_API_KEY"); key != "" {
		secrets.APIKeys["nvidia"] = key
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		secrets.APIKeys["anthropic"] = key
	}
	if key := os.Getenv("TOGETHER_API_KEY"); key != "" {
		secrets.APIKeys["together"] = key
	}

	// Load Hugging Face token
	secrets.HuggingFaceToken = os.Getenv("HUGGING_FACE_TOKEN")

	return secrets, nil
}

// GetAPIKey returns the API key for a given base URL
func (s *Secrets) GetAPIKey(baseURL string) string {
	// Try to match common provider domains (provider-specific keys)
	if contains(baseURL, "openai.com") {
		if key := s.APIKeys["openai"]; key != "" {
			return key
		}
	}
	if contains(baseURL, "nvidia.com") {
		if key := s.APIKeys["nvidia"]; key != "" {
			return key
		}
	}
	if contains(baseURL, "anthropic.com") {
		if key := s.APIKeys["anthropic"]; key != "" {
			return key
		}
	}
	if contains(baseURL, "together.xyz") || contains(baseURL, "together.ai") {
		if key := s.APIKeys["together"]; key != "" {
			return key
		}
	}

	// Fall back to generic API_KEY for any OpenAI-compatible provider
	if key := s.APIKeys["generic"]; key != "" {
		return key
	}

	// If no key found, return empty (could be local server without auth)
	return ""
}

// GetProviderName extracts a provider name from a base URL for rate limiting
func GetProviderName(baseURL string) string {
	// Match common provider domains
	if contains(baseURL, "openai.com") {
		return "openai"
	}
	if contains(baseURL, "nvidia.com") {
		return "nvidia"
	}
	if contains(baseURL, "anthropic.com") {
		return "anthropic"
	}
	if contains(baseURL, "together.xyz") || contains(baseURL, "together.ai") {
		return "together"
	}
	// For localhost or unknown providers, use the full base URL as provider name
	return baseURL
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
