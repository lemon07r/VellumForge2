package config

import (
	"fmt"
	"os"
)

// Config represents the complete application configuration
type Config struct {
	Generation      GenerationConfig       `toml:"generation"`
	Models          map[string]ModelConfig `toml:"models"`
	PromptTemplates PromptTemplates        `toml:"prompt_templates"`
	HuggingFace     HuggingFaceConfig      `toml:"huggingface"`
}

// GenerationConfig holds generation-specific settings
type GenerationConfig struct {
	MainTopic             string `toml:"main_topic"`
	NumSubtopics          int    `toml:"num_subtopics"`
	NumPromptsPerSubtopic int    `toml:"num_prompts_per_subtopic"`
	Concurrency           int    `toml:"concurrency"`
}

// ModelConfig represents configuration for a single model endpoint
type ModelConfig struct {
	BaseURL            string  `toml:"base_url"`
	ModelName          string  `toml:"model_name"`
	Temperature        float64 `toml:"temperature"`
	TopP               float64 `toml:"top_p"`
	TopK               int     `toml:"top_k"`
	MinP               float64 `toml:"min_p"`
	MaxOutputTokens    int     `toml:"max_output_tokens"`
	ContextSize        int     `toml:"context_size"`
	RateLimitPerMinute int     `toml:"rate_limit_per_minute"`
	Enabled            bool    `toml:"enabled"` // Only used for judge model
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

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate generation config
	if c.Generation.MainTopic == "" {
		return fmt.Errorf("generation.main_topic is required")
	}
	if c.Generation.NumSubtopics < 1 {
		return fmt.Errorf("generation.num_subtopics must be at least 1")
	}
	if c.Generation.NumPromptsPerSubtopic < 1 {
		return fmt.Errorf("generation.num_prompts_per_subtopic must be at least 1")
	}
	if c.Generation.Concurrency < 1 {
		return fmt.Errorf("generation.concurrency must be at least 1")
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
	return nil
}

// LoadSecrets loads sensitive credentials from environment variables
func LoadSecrets() (*Secrets, error) {
	secrets := &Secrets{
		APIKeys: make(map[string]string),
	}

	// Load common API keys
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
	// Try to match common provider domains
	if contains(baseURL, "openai.com") {
		return s.APIKeys["openai"]
	}
	if contains(baseURL, "nvidia.com") {
		return s.APIKeys["nvidia"]
	}
	if contains(baseURL, "anthropic.com") {
		return s.APIKeys["anthropic"]
	}
	if contains(baseURL, "together.xyz") || contains(baseURL, "together.ai") {
		return s.APIKeys["together"]
	}

	// If no match, return empty (could be local server)
	return ""
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findInString(s, substr)))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
