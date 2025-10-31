package config

import (
	"fmt"
	"net/url"
	"unicode"
)

const (
	// MaxMainTopicLength is the maximum allowed length for main topic
	MaxMainTopicLength = 500

	// MaxModelNameLength is the maximum allowed length for model names
	MaxModelNameLength = 100

	// MaxTemplateSize is the maximum allowed size for template content
	MaxTemplateSize = 50 * 1024 // 50KB
)

// ValidateInputs performs additional security validation on user-controllable fields.
// This prevents potential DoS attacks, injection attacks, and other security issues.
func (c *Config) ValidateInputs() error {
	// Validate MainTopic
	if err := validateMainTopic(c.Generation.MainTopic); err != nil {
		return fmt.Errorf("invalid main_topic: %w", err)
	}

	// Validate model configurations
	for name, mc := range c.Models {
		if err := validateModelName(mc.ModelName, name); err != nil {
			return err
		}

		if err := validateBaseURL(mc.BaseURL, name); err != nil {
			return err
		}
	}

	// Validate template sizes
	if err := c.validateTemplateSizes(); err != nil {
		return err
	}

	return nil
}

// validateMainTopic checks the main topic for security issues
func validateMainTopic(mainTopic string) error {
	// Check length
	if len(mainTopic) > MaxMainTopicLength {
		return fmt.Errorf("exceeds maximum length of %d characters (got %d)",
			MaxMainTopicLength, len(mainTopic))
	}

	// Check for control characters (except newlines and tabs)
	if containsControlChars(mainTopic) {
		return fmt.Errorf("contains invalid control characters")
	}

	return nil
}

// validateModelName checks model name for security issues
func validateModelName(modelName, configKey string) error {
	if len(modelName) > MaxModelNameLength {
		return fmt.Errorf("model '%s' name exceeds maximum length of %d (got %d)",
			configKey, MaxModelNameLength, len(modelName))
	}

	// Check for control characters
	if containsControlChars(modelName) {
		return fmt.Errorf("model '%s' name contains invalid control characters", configKey)
	}

	return nil
}

// validateBaseURL checks that the base URL is properly formatted and safe
func validateBaseURL(baseURL, configKey string) error {
	// Parse URL
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("model '%s' has invalid base_url: %w", configKey, err)
	}

	// Check scheme
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("model '%s' base_url must use http or https scheme (got %s)",
			configKey, u.Scheme)
	}

	// Check host is present
	if u.Host == "" {
		return fmt.Errorf("model '%s' base_url must have a host", configKey)
	}

	return nil
}

// validateTemplateSizes checks that templates are within reasonable size limits
func (c *Config) validateTemplateSizes() error {
	templates := []struct {
		name  string
		value string
	}{
		{"subtopic_generation", c.PromptTemplates.SubtopicGeneration},
		{"prompt_generation", c.PromptTemplates.PromptGeneration},
		{"chosen_generation", c.PromptTemplates.ChosenGeneration},
		{"rejected_generation", c.PromptTemplates.RejectedGeneration},
		{"judge_rubric", c.PromptTemplates.JudgeRubric},
	}

	for _, tmpl := range templates {
		if len(tmpl.value) > MaxTemplateSize {
			return fmt.Errorf("template '%s' exceeds maximum size of %d bytes (got %d)",
				tmpl.name, MaxTemplateSize, len(tmpl.value))
		}
	}

	return nil
}

// containsControlChars checks if a string contains control characters
// (excluding newlines, tabs, and carriage returns which are acceptable)
func containsControlChars(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' {
			return true
		}
	}
	return false
}
