package config

import (
	"strings"
	"testing"

	"github.com/lamim/vellumforge2/pkg/models"
)

func TestValidateUpperBounds(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "concurrency too high",
			cfg: Config{
				Generation: GenerationConfig{
					MainTopic:             "Test",
					NumSubtopics:          2,
					NumPromptsPerSubtopic: 2,
					Concurrency:           2000, // > 1024
				},
				Models: map[string]ModelConfig{
					"main": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
					"rejected": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test2",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
				},
				PromptTemplates: PromptTemplates{
					SubtopicGeneration: "test",
					PromptGeneration:   "test",
				},
			},
			wantErr: true,
			errMsg:  "concurrency must not exceed",
		},
		{
			name: "num_subtopics too high",
			cfg: Config{
				Generation: GenerationConfig{
					MainTopic:             "Test",
					NumSubtopics:          15000, // > 10000
					NumPromptsPerSubtopic: 2,
					Concurrency:           8,
				},
				Models: map[string]ModelConfig{
					"main": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
					"rejected": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test2",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
				},
				PromptTemplates: PromptTemplates{
					SubtopicGeneration: "test",
					PromptGeneration:   "test",
				},
			},
			wantErr: true,
			errMsg:  "num_subtopics must not exceed",
		},
		{
			name: "num_prompts_per_subtopic too high",
			cfg: Config{
				Generation: GenerationConfig{
					MainTopic:             "Test",
					NumSubtopics:          2,
					NumPromptsPerSubtopic: 20000, // > 10000
					Concurrency:           8,
				},
				Models: map[string]ModelConfig{
					"main": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
					"rejected": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test2",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
				},
				PromptTemplates: PromptTemplates{
					SubtopicGeneration: "test",
					PromptGeneration:   "test",
				},
			},
			wantErr: true,
			errMsg:  "num_prompts_per_subtopic must not exceed",
		},
		{
			name: "max_output_tokens > context_size",
			cfg: Config{
				Generation: GenerationConfig{
					MainTopic:             "Test",
					NumSubtopics:          2,
					NumPromptsPerSubtopic: 2,
					Concurrency:           8,
				},
				Models: map[string]ModelConfig{
					"main": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    32000,
						ContextSize:        16000, // MaxOutputTokens > ContextSize
						RateLimitPerMinute: 60,
					},
					"rejected": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test2",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
				},
				PromptTemplates: PromptTemplates{
					SubtopicGeneration: "test",
					PromptGeneration:   "test",
				},
			},
			wantErr: true,
			errMsg:  "must not exceed context_size",
		},
		{
			name: "over_generation_buffer out of range (negative)",
			cfg: Config{
				Generation: GenerationConfig{
					MainTopic:             "Test",
					NumSubtopics:          2,
					NumPromptsPerSubtopic: 2,
					Concurrency:           8,
					OverGenerationBuffer:  -0.1, // < 0.0
				},
				Models: map[string]ModelConfig{
					"main": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
					"rejected": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test2",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
				},
				PromptTemplates: PromptTemplates{
					SubtopicGeneration: "test",
					PromptGeneration:   "test",
				},
			},
			wantErr: true,
			errMsg:  "must be between 0.0 and 1.0",
		},
		{
			name: "over_generation_buffer out of range (too high)",
			cfg: Config{
				Generation: GenerationConfig{
					MainTopic:             "Test",
					NumSubtopics:          2,
					NumPromptsPerSubtopic: 2,
					Concurrency:           8,
					OverGenerationBuffer:  1.5, // > 1.0
				},
				Models: map[string]ModelConfig{
					"main": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
					"rejected": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test2",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
				},
				PromptTemplates: PromptTemplates{
					SubtopicGeneration: "test",
					PromptGeneration:   "test",
				},
			},
			wantErr: true,
			errMsg:  "must be between 0.0 and 1.0",
		},
		{
			name: "valid config with all limits at max",
			cfg: Config{
				Generation: GenerationConfig{
					MainTopic:             "Test",
					NumSubtopics:          10000, // At max
					NumPromptsPerSubtopic: 10000, // At max
					Concurrency:           1024,  // At max
					OverGenerationBuffer:  1.0,   // At max
					MaxExclusionListSize:  50,
					DatasetMode:           models.DatasetModeDPO, // DPO mode doesn't require judge
				},
				Models: map[string]ModelConfig{
					"main": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    2048,
						ContextSize:        2048, // Equal is OK
						RateLimitPerMinute: 60,
					},
					"rejected": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test2",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
				},
				PromptTemplates: PromptTemplates{
					SubtopicGeneration: "test",
					PromptGeneration:   "test",
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with all limits at min",
			cfg: Config{
				Generation: GenerationConfig{
					MainTopic:             "Test",
					NumSubtopics:          1,
					NumPromptsPerSubtopic: 1,
					Concurrency:           1,
					OverGenerationBuffer:  0.0,
					MaxExclusionListSize:  1,
					DatasetMode:           models.DatasetModeDPO, // DPO mode doesn't require judge
				},
				Models: map[string]ModelConfig{
					"main": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1,
						ContextSize:        100,
						RateLimitPerMinute: 60,
					},
					"rejected": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test2",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1,
						ContextSize:        100,
						RateLimitPerMinute: 60,
					},
				},
				PromptTemplates: PromptTemplates{
					SubtopicGeneration: "test",
					PromptGeneration:   "test",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Error message = %v, should contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

// Note: Default values are tested through the Load() function in config_test.go
// since applyDefaults() is not exported

func TestDisableValidationLimits(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "exceed limits with validation disabled",
			cfg: Config{
				Generation: GenerationConfig{
					MainTopic:               "Test",
					NumSubtopics:            50000, // > 10000 but allowed
					NumPromptsPerSubtopic:   20000, // > 10000 but allowed
					Concurrency:             2048,  // > 1024 but allowed
					DisableValidationLimits: true,  // Validation disabled
					DatasetMode:             models.DatasetModeDPO, // DPO mode doesn't require judge
				},
				Models: map[string]ModelConfig{
					"main": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
					"rejected": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test2",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
				},
				PromptTemplates: PromptTemplates{
					SubtopicGeneration: "test",
					PromptGeneration:   "test",
				},
			},
			wantErr: false, // Should pass with disabled validation
		},
		{
			name: "exceed limits with validation enabled",
			cfg: Config{
				Generation: GenerationConfig{
					MainTopic:               "Test",
					NumSubtopics:            50000, // > 10000
					NumPromptsPerSubtopic:   20000, // > 10000
					Concurrency:             2048,  // > 1024
					DisableValidationLimits: false, // Validation enabled
				},
				Models: map[string]ModelConfig{
					"main": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
					"rejected": {
						BaseURL:            "https://api.example.com",
						ModelName:          "test2",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
				},
				PromptTemplates: PromptTemplates{
					SubtopicGeneration: "test",
					PromptGeneration:   "test",
				},
			},
			wantErr: true, // Should fail - validation enabled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
