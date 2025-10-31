package config

import (
	"strings"
	"testing"
)

func TestValidateMainTopic_Valid(t *testing.T) {
	tests := []string{
		"Artificial Intelligence",
		"Machine Learning in Healthcare",
		"Quantum Computing Applications",
		"Climate Change and Sustainability",
		"Space Exploration\nand Discovery", // Newlines are OK
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			if err := validateMainTopic(tt); err != nil {
				t.Errorf("validateMainTopic(%q) returned unexpected error: %v", tt, err)
			}
		})
	}
}

func TestValidateMainTopic_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // substring of expected error
	}{
		{
			name:  "too_long",
			input: strings.Repeat("a", MaxMainTopicLength+1),
			want:  "exceeds maximum length",
		},
		{
			name:  "control_chars",
			input: "Test\x00Topic", // Null byte
			want:  "invalid control characters",
		},
		{
			name:  "bell_char",
			input: "Test\x07Topic", // Bell character
			want:  "invalid control characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMainTopic(tt.input)
			if err == nil {
				t.Errorf("validateMainTopic(%q) expected error, got nil", tt.input)
			} else if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("validateMainTopic(%q) error = %v, want substring %q", tt.input, err, tt.want)
			}
		})
	}
}

func TestValidateModelName_Valid(t *testing.T) {
	tests := []string{
		"gpt-4",
		"llama-3.1-70b-instruct",
		"claude-3-opus-20240229",
		"mixtral-8x7b-v0.1",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			if err := validateModelName(tt, "test"); err != nil {
				t.Errorf("validateModelName(%q) returned unexpected error: %v", tt, err)
			}
		})
	}
}

func TestValidateModelName_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "too_long",
			input: strings.Repeat("a", MaxModelNameLength+1),
			want:  "exceeds maximum length",
		},
		{
			name:  "control_chars",
			input: "model\x00name",
			want:  "invalid control characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModelName(tt.input, "test")
			if err == nil {
				t.Errorf("validateModelName(%q) expected error, got nil", tt.input)
			} else if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("validateModelName(%q) error = %v, want substring %q", tt.input, err, tt.want)
			}
		})
	}
}

func TestValidateBaseURL_Valid(t *testing.T) {
	tests := []string{
		"https://api.openai.com/v1",
		"http://localhost:8080",
		"https://api.anthropic.com",
		"http://192.168.1.100:11434",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			if err := validateBaseURL(tt, "test"); err != nil {
				t.Errorf("validateBaseURL(%q) returned unexpected error: %v", tt, err)
			}
		})
	}
}

func TestValidateBaseURL_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "invalid_scheme",
			input: "ftp://example.com",
			want:  "must use http or https scheme",
		},
		{
			name:  "missing_scheme",
			input: "example.com",
			want:  "must use http or https scheme",
		},
		{
			name:  "no_host",
			input: "https://",
			want:  "must have a host",
		},
		{
			name:  "invalid_url",
			input: "ht!tp://invalid",
			want:  "invalid base_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBaseURL(tt.input, "test")
			if err == nil {
				t.Errorf("validateBaseURL(%q) expected error, got nil", tt.input)
			} else if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("validateBaseURL(%q) error = %v, want substring %q", tt.input, err, tt.want)
			}
		})
	}
}

func TestValidateTemplateSizes(t *testing.T) {
	// Valid: small templates
	cfg := &Config{
		PromptTemplates: PromptTemplates{
			SubtopicGeneration: "Small template",
			PromptGeneration:   "Another small template",
			ChosenGeneration:   "Small",
			RejectedGeneration: "Small",
			JudgeRubric:        "Small",
		},
	}

	if err := cfg.validateTemplateSizes(); err != nil {
		t.Errorf("validateTemplateSizes() with small templates returned error: %v", err)
	}

	// Invalid: oversized template
	cfg.PromptTemplates.SubtopicGeneration = strings.Repeat("x", MaxTemplateSize+1)
	err := cfg.validateTemplateSizes()
	if err == nil {
		t.Error("validateTemplateSizes() with oversized template expected error, got nil")
	} else if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("validateTemplateSizes() error = %v, want substring 'exceeds maximum size'", err)
	}
}

func TestContainsControlChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "normal_text",
			input: "Hello World",
			want:  false,
		},
		{
			name:  "with_newline",
			input: "Hello\nWorld",
			want:  false, // Newlines are allowed
		},
		{
			name:  "with_tab",
			input: "Hello\tWorld",
			want:  false, // Tabs are allowed
		},
		{
			name:  "with_carriage_return",
			input: "Hello\rWorld",
			want:  false, // CR is allowed
		},
		{
			name:  "null_byte",
			input: "Hello\x00World",
			want:  true, // Null byte is control char
		},
		{
			name:  "bell_char",
			input: "Hello\x07World",
			want:  true, // Bell is control char
		},
		{
			name:  "escape_char",
			input: "Hello\x1bWorld",
			want:  true, // ESC is control char
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsControlChars(tt.input)
			if got != tt.want {
				t.Errorf("containsControlChars(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateInputs_Integration(t *testing.T) {
	// Valid configuration
	cfg := &Config{
		Generation: GenerationConfig{
			MainTopic: "Test Topic",
		},
		Models: map[string]ModelConfig{
			"main": {
				BaseURL:   "https://api.example.com",
				ModelName: "gpt-4",
			},
		},
		PromptTemplates: PromptTemplates{
			SubtopicGeneration: "Template 1",
			PromptGeneration:   "Template 2",
			ChosenGeneration:   "Template 3",
			RejectedGeneration: "Template 4",
			JudgeRubric:        "Template 5",
		},
	}

	if err := cfg.ValidateInputs(); err != nil {
		t.Errorf("ValidateInputs() with valid config returned error: %v", err)
	}

	// Invalid: bad main topic
	cfgBad := *cfg
	cfgBad.Generation.MainTopic = strings.Repeat("a", MaxMainTopicLength+1)
	if err := cfgBad.ValidateInputs(); err == nil {
		t.Error("ValidateInputs() with oversized main_topic expected error, got nil")
	}

	// Invalid: bad URL
	cfgBad = *cfg
	cfgBad.Models["main"] = ModelConfig{
		BaseURL:   "ftp://invalid.com",
		ModelName: "model",
	}
	if err := cfgBad.ValidateInputs(); err == nil {
		t.Error("ValidateInputs() with invalid URL expected error, got nil")
	}
}
