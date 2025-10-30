package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// Load reads and parses the configuration file and environment variables
func Load(configPath string) (*Config, *Secrets, error) {
	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse TOML
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults
	applyDefaults(&cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Additional input security validation
	if err := cfg.ValidateInputs(); err != nil {
		return nil, nil, fmt.Errorf("input validation failed: %w", err)
	}

	// Load secrets from environment
	secrets, err := LoadSecrets()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load secrets: %w", err)
	}

	return &cfg, secrets, nil
}

// applyDefaults sets default values for optional configuration fields
func applyDefaults(cfg *Config) {
	// Generation defaults
	if cfg.Generation.Concurrency == 0 {
		cfg.Generation.Concurrency = 8
	}
	if cfg.Generation.OverGenerationBuffer == 0 {
		cfg.Generation.OverGenerationBuffer = 0.15 // 15% buffer by default
	}
	if cfg.Generation.MaxExclusionListSize == 0 {
		cfg.Generation.MaxExclusionListSize = 50
	}

	// Apply defaults for each model
	for name, model := range cfg.Models {
		if model.Temperature == 0 {
			model.Temperature = 0.7
		}
		if model.TopP == 0 {
			model.TopP = 1.0
		}
		if model.TopK == 0 {
			model.TopK = -1 // -1 means disabled
		}
		if model.MinP == 0 {
			model.MinP = 0.0
		}
		if model.MaxOutputTokens == 0 {
			model.MaxOutputTokens = 4096
		}
		if model.ContextSize == 0 {
			model.ContextSize = 16384
		}
		if model.RateLimitPerMinute == 0 {
			model.RateLimitPerMinute = 60
		}
		if model.MaxBackoffSeconds == 0 {
			model.MaxBackoffSeconds = 120 // 2 minutes default
		}
		// Default max_retries is 3
		// NOTE: In TOML, we can't distinguish 0 from unset, so:
		// - Unset (0) → defaults to 3
		// - Explicitly set to -1 → unlimited retries
		// - Any positive number → use that value
		if model.MaxRetries == 0 {
			model.MaxRetries = 3 // Default to 3 retries
		}
		// If structure_temperature not set, it will use regular temperature (0 = unset)
		
		// Default judge timeout: 100 seconds (generous for slower models)
		if model.JudgeTimeoutSeconds == 0 {
			model.JudgeTimeoutSeconds = 100
		}
		
		cfg.Models[name] = model
	}

	// Apply default for subtopic chunk size
	if cfg.Generation.SubtopicChunkSize == 0 {
		cfg.Generation.SubtopicChunkSize = 30 // Default chunk size
	}

	// Apply default templates if not provided
	if cfg.PromptTemplates.SubtopicGeneration == "" {
		cfg.PromptTemplates.SubtopicGeneration = GetDefaultSubtopicTemplate()
	}
	if cfg.PromptTemplates.PromptGeneration == "" {
		cfg.PromptTemplates.PromptGeneration = GetDefaultPromptTemplate()
	}
	if cfg.PromptTemplates.JudgeRubric == "" {
		cfg.PromptTemplates.JudgeRubric = GetDefaultJudgeTemplate()
	}
}
