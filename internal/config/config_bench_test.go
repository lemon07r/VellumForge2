package config

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkLoad benchmarks config loading
func BenchmarkLoad(b *testing.B) {
	// Create a temporary config file
	tempDir := b.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")

	configContent := `
[generation]
main_topic = "Test Topic"
num_subtopics = 5
num_prompts_per_subtopic = 10
concurrency = 8
dataset_mode = "dpo"

[models.main]
base_url = "https://api.example.com/v1"
model_name = "test-model"
temperature = 0.7
top_p = 1.0
max_output_tokens = 1024
context_size = 2048
rate_limit_per_minute = 60

[models.rejected]
base_url = "https://api.example.com/v1"
model_name = "test-model-2"
temperature = 0.8
top_p = 1.0
max_output_tokens = 1024
context_size = 2048
rate_limit_per_minute = 60

[prompt_templates]
subtopic_generation = "Generate {{.Count}} subtopics for {{.MainTopic}}"
prompt_generation = "Generate {{.Count}} prompts for {{.Subtopic}}"
chosen_generation = "Generate chosen response"
rejected_generation = "Generate rejected response"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		b.Fatal(err)
	}

	// Set environment variables
	if err := os.Setenv("OPENAI_API_KEY", "test-key-123"); err != nil {
		b.Fatal(err)
	}
	defer func() {
		_ = os.Unsetenv("OPENAI_API_KEY")
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := Load(configPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidate benchmarks config validation
func BenchmarkValidate(b *testing.B) {
	cfg := &Config{
		Generation: GenerationConfig{
			MainTopic:             "Test",
			NumSubtopics:          10,
			NumPromptsPerSubtopic: 10,
			Concurrency:           8,
			DatasetMode:           "dpo",
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
			ChosenGeneration:   "test",
			RejectedGeneration: "test",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValidateInputs benchmarks input validation
func BenchmarkValidateInputs(b *testing.B) {
	cfg := &Config{
		Generation: GenerationConfig{
			MainTopic:             "Test Topic",
			NumSubtopics:          10,
			NumPromptsPerSubtopic: 10,
			Concurrency:           8,
		},
		Models: map[string]ModelConfig{
			"main": {
				BaseURL:   "https://api.example.com",
				ModelName: "test-model-name",
			},
		},
		PromptTemplates: PromptTemplates{
			SubtopicGeneration: "Generate {{.Count}} subtopics",
			PromptGeneration:   "Generate {{.Count}} prompts",
			ChosenGeneration:   "Generate chosen response",
			RejectedGeneration: "Generate rejected response",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.ValidateInputs(); err != nil {
			b.Fatal(err)
		}
	}
}
