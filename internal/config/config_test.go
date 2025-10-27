package config

import (
	"os"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Generation: GenerationConfig{
					MainTopic:             "Test Topic",
					NumSubtopics:          2,
					NumPromptsPerSubtopic: 2,
					Concurrency:           4,
				},
				Models: map[string]ModelConfig{
					"main": {
						BaseURL:            "https://api.example.com/v1",
						ModelName:          "test-model",
						Temperature:        0.7,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
					"rejected": {
						BaseURL:            "https://api.example.com/v1",
						ModelName:          "test-model-2",
						Temperature:        0.8,
						TopP:               1.0,
						MaxOutputTokens:    1024,
						ContextSize:        2048,
						RateLimitPerMinute: 60,
					},
				},
				PromptTemplates: PromptTemplates{
					SubtopicGeneration: "template1",
					PromptGeneration:   "template2",
				},
			},
			wantErr: false,
		},
		{
			name: "missing main topic",
			cfg: Config{
				Generation: GenerationConfig{
					NumSubtopics:          2,
					NumPromptsPerSubtopic: 2,
					Concurrency:           4,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid concurrency",
			cfg: Config{
				Generation: GenerationConfig{
					MainTopic:             "Test",
					NumSubtopics:          2,
					NumPromptsPerSubtopic: 2,
					Concurrency:           0,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadSecrets(t *testing.T) {
	// Set test environment variables
	if err := os.Setenv("OPENAI_API_KEY", "test-key-123"); err != nil {
		t.Fatalf("Failed to set OPENAI_API_KEY: %v", err)
	}
	if err := os.Setenv("NVIDIA_API_KEY", "test-nvidia-key"); err != nil {
		t.Fatalf("Failed to set NVIDIA_API_KEY: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("OPENAI_API_KEY")
		_ = os.Unsetenv("NVIDIA_API_KEY")
	}()

	secrets, err := LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() error = %v", err)
	}

	if secrets.APIKeys["openai"] != "test-key-123" {
		t.Errorf("Expected OpenAI key to be 'test-key-123', got %s", secrets.APIKeys["openai"])
	}

	if secrets.APIKeys["nvidia"] != "test-nvidia-key" {
		t.Errorf("Expected NVIDIA key to be 'test-nvidia-key', got %s", secrets.APIKeys["nvidia"])
	}
}

func TestGetAPIKey(t *testing.T) {
	secrets := &Secrets{
		APIKeys: map[string]string{
			"openai": "openai-key",
			"nvidia": "nvidia-key",
		},
	}

	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			name:    "OpenAI URL",
			baseURL: "https://api.openai.com/v1",
			want:    "openai-key",
		},
		{
			name:    "NVIDIA URL",
			baseURL: "https://integrate.api.nvidia.com/v1",
			want:    "nvidia-key",
		},
		{
			name:    "Unknown URL",
			baseURL: "https://unknown.com/v1",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := secrets.GetAPIKey(tt.baseURL)
			if got != tt.want {
				t.Errorf("GetAPIKey() = %v, want %v", got, tt.want)
			}
		})
	}
}
