package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/lamim/vellumforge2/pkg/models"
)

// JudgeFilteringConfig holds optional judge-based filtering settings
type JudgeFilteringConfig struct {
	Enabled          bool    `toml:"enabled"`            // Enable judge-based filtering
	UseExplanations  bool    `toml:"use_explanations"`   // Include reasoning in judge responses (false = scores only)
	MinChosenScore   float64 `toml:"min_chosen_score"`   // Minimum average score for chosen responses (1.0-5.0)
	MaxRejectedScore float64 `toml:"max_rejected_score"` // Maximum average score for rejected responses (1.0-5.0)
}

// Config represents the complete application configuration
type Config struct {
	Generation           GenerationConfig       `toml:"generation"`
	Models               map[string]ModelConfig `toml:"models"`
	PromptTemplates      PromptTemplates        `toml:"prompt_templates"`
	HuggingFace          HuggingFaceConfig      `toml:"huggingface"`
	ProviderRateLimits   map[string]int         `toml:"provider_rate_limits"`   // Global rate limits per provider (requests per minute)
	ProviderBurstPercent int                    `toml:"provider_burst_percent"` // Burst capacity as percentage (1-50, default: 15)
	JudgeFiltering       JudgeFilteringConfig   `toml:"judge_filtering"`        // Optional judge-based quality filtering
}

// GenerationConfig holds generation-specific settings
type GenerationConfig struct {
	MainTopic                string             `toml:"main_topic"`
	NumSubtopics             int                `toml:"num_subtopics"`
	SubtopicChunkSize        int                `toml:"subtopic_chunk_size"` // Request subtopics in chunks (0=all at once, default: 30)
	NumPromptsPerSubtopic    int                `toml:"num_prompts_per_subtopic"`
	Concurrency              int                `toml:"concurrency"`
	OverGenerationBuffer     float64            `toml:"over_generation_buffer"`     // Buffer percentage (0.0-1.0, default 0.15)
	MaxExclusionListSize     int                `toml:"max_exclusion_list_size"`    // Max items in exclusion list (default 50)
	MinSuccessRate           float64            `toml:"min_success_rate"`           // Minimum success rate for prompt generation (0.0-1.0, default 0.90)
	PromptRetryAttempts      int                `toml:"prompt_retry_attempts"`      // Number of retry attempts for failed subtopics (default 2)
	DisableValidationLimits  bool               `toml:"disable_validation_limits"`  // Disable upper bound validation (use with caution)
	EnableCheckpointing      bool               `toml:"enable_checkpointing"`       // Enable checkpoint/resume support
	CheckpointInterval       int                `toml:"checkpoint_interval"`        // Save checkpoint every N completed jobs (default: 10)
	ResumeFromSession        string             `toml:"resume_from_session"`        // Session directory to resume from (e.g., "session_2025-10-27T12-34-56")
	DatasetMode              models.DatasetMode `toml:"dataset_mode"`               // Dataset format: sft, dpo, kto, mo-dpo (default: mo-dpo)
	IncludeTopicColumns      bool               `toml:"include_topic_columns"`      // For SFT mode: include main_topic/sub_topic columns (default: true)
	EnableReasoningCapture   bool               `toml:"enable_reasoning_capture"`   // Capture reasoning from reasoning models (creates dual datasets)
	ReasoningCaptureRejected bool               `toml:"reasoning_capture_rejected"` // Also capture reasoning for rejected responses (default: false)
}

// ModelConfig represents configuration for a single model endpoint
type ModelConfig struct {
	BaseURL              string  `toml:"base_url"`
	ModelName            string  `toml:"model_name"`
	Temperature          float64 `toml:"temperature"`
	StructureTemperature float64 `toml:"structure_temperature"` // Temperature for JSON generation (optional, defaults to temperature)
	TopP                 float64 `toml:"top_p"`
	MaxOutputTokens      int     `toml:"max_output_tokens"`
	ContextSize          int     `toml:"context_size"`
	RateLimitPerMinute   int     `toml:"rate_limit_per_minute"`
	MaxBackoffSeconds    int     `toml:"max_backoff_seconds"`             // Optional: max backoff duration (default 120)
	MaxRetries           int     `toml:"max_retries"`                     // Optional: max retry attempts (default 3, 0 = unlimited)
	HTTPTimeoutSeconds   int     `toml:"http_timeout_seconds"`            // Optional: HTTP request timeout (default 120, 0 = no timeout)
	JudgeTimeoutSeconds  int     `toml:"judge_timeout_seconds,omitempty"` // Timeout for judge API calls (default: 100s)
	UseJSONMode          bool    `toml:"use_json_mode"`                   // Enable structured JSON output mode (optional)
	UseStreaming         bool    `toml:"use_streaming"`                   // Enable streaming mode (bypasses gateway timeouts, default: false)
	Enabled              bool    `toml:"enabled"`                         // Only used for judge model
}

// PromptTemplates holds all customizable prompt templates
type PromptTemplates struct {
	SubtopicGeneration   string `toml:"subtopic_generation"`
	PromptGeneration     string `toml:"prompt_generation"`
	ChosenGeneration     string `toml:"chosen_generation"`
	RejectedGeneration   string `toml:"rejected_generation"`
	JudgeRubric          string `toml:"judge_rubric"`
	ChosenSystemPrompt   string `toml:"chosen_system_prompt"`   // Optional system prompt for chosen generation
	RejectedSystemPrompt string `toml:"rejected_system_prompt"` // Optional system prompt for rejected generation
	SubtopicSystemPrompt string `toml:"subtopic_system_prompt"` // Optional system prompt for subtopic generation
	PromptSystemPrompt   string `toml:"prompt_system_prompt"`   // Optional system prompt for prompt generation
	JudgeSystemPrompt    string `toml:"judge_system_prompt"`    // Optional system prompt for judge evaluation
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

	// Set default dataset mode if not specified
	if c.Generation.DatasetMode == "" {
		c.Generation.DatasetMode = models.DatasetModeMODPO // Default to current behavior
	}
	// Validate dataset mode
	validModes := []models.DatasetMode{models.DatasetModeSFT, models.DatasetModeDPO, models.DatasetModeKTO, models.DatasetModeMODPO}
	validMode := false
	for _, mode := range validModes {
		if c.Generation.DatasetMode == mode {
			validMode = true
			break
		}
	}
	if !validMode {
		return fmt.Errorf("generation.dataset_mode must be one of: sft, dpo, kto, mo-dpo (got %s)", c.Generation.DatasetMode)
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
	if c.Generation.MinSuccessRate < 0 || c.Generation.MinSuccessRate > 1.0 {
		return fmt.Errorf("generation.min_success_rate must be between 0.0 and 1.0 (got %.2f)", c.Generation.MinSuccessRate)
	}
	if c.Generation.PromptRetryAttempts < 0 || c.Generation.PromptRetryAttempts > 5 {
		return fmt.Errorf("generation.prompt_retry_attempts must be between 0 and 5 (got %d)", c.Generation.PromptRetryAttempts)
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

	// Validate rejected model exists (unless SFT mode)
	rejectedModel, ok := c.Models["rejected"]
	if !ok {
		if c.Generation.DatasetMode != models.DatasetModeSFT {
			return fmt.Errorf("models.rejected is required for dataset_mode=%s", c.Generation.DatasetMode)
		}
		// Warn if missing in SFT mode
		fmt.Fprintf(os.Stderr, "WARNING: models.rejected not configured for SFT mode - only chosen responses will be generated\n")
	} else {
		if err := validateModelConfig("rejected", rejectedModel); err != nil {
			return err
		}
	}

	// Validate judge model if enabled
	judgeModel, judgeExists := c.Models["judge"]
	if judgeExists && judgeModel.Enabled {
		if err := validateModelConfig("judge", judgeModel); err != nil {
			return err
		}
		if c.PromptTemplates.JudgeRubric == "" {
			return fmt.Errorf("prompt_templates.judge_rubric is required when judge is enabled")
		}
	}

	// MO-DPO mode requires judge
	if c.Generation.DatasetMode == models.DatasetModeMODPO {
		if !judgeExists || !judgeModel.Enabled {
			return fmt.Errorf("dataset_mode=mo-dpo requires models.judge with enabled=true")
		}
	}

	// Validate judge filtering config
	if c.JudgeFiltering.Enabled {
		if !judgeExists || !judgeModel.Enabled {
			return fmt.Errorf("judge_filtering.enabled=true requires models.judge with enabled=true")
		}
		if c.JudgeFiltering.MinChosenScore < 1.0 || c.JudgeFiltering.MinChosenScore > 5.0 {
			return fmt.Errorf("judge_filtering.min_chosen_score must be between 1.0 and 5.0 (got %.2f)", c.JudgeFiltering.MinChosenScore)
		}
		if c.JudgeFiltering.MaxRejectedScore < 1.0 || c.JudgeFiltering.MaxRejectedScore > 5.0 {
			return fmt.Errorf("judge_filtering.max_rejected_score must be between 1.0 and 5.0 (got %.2f)", c.JudgeFiltering.MaxRejectedScore)
		}
		// Set default thresholds if not specified
		if c.JudgeFiltering.MinChosenScore == 0 {
			c.JudgeFiltering.MinChosenScore = 4.0
		}
		if c.JudgeFiltering.MaxRejectedScore == 0 {
			c.JudgeFiltering.MaxRejectedScore = 3.0
		}
	}

	// Warn if judge filtering is enabled in MO-DPO mode (redundant)
	if c.Generation.DatasetMode == models.DatasetModeMODPO && c.JudgeFiltering.Enabled {
		fmt.Fprintf(os.Stderr, "WARNING: judge_filtering is redundant in mo-dpo mode (judge scoring is always included)\n")
	}

	// Validate prompt templates
	if c.PromptTemplates.SubtopicGeneration == "" {
		return fmt.Errorf("prompt_templates.subtopic_generation is required")
	}
	if c.PromptTemplates.PromptGeneration == "" {
		return fmt.Errorf("prompt_templates.prompt_generation is required")
	}
	if c.PromptTemplates.ChosenGeneration == "" {
		return fmt.Errorf("prompt_templates.chosen_generation is required")
	}
	if c.PromptTemplates.RejectedGeneration == "" {
		return fmt.Errorf("prompt_templates.rejected_generation is required")
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
	if mc.StructureTemperature > 0 && (mc.StructureTemperature < 0 || mc.StructureTemperature > 2) {
		return fmt.Errorf("models.%s.structure_temperature must be between 0 and 2", name)
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
