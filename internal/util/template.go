package util

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// RenderTemplate renders a template string with the given data
// Includes validation to prevent template injection attacks
func RenderTemplate(tmpl string, data map[string]interface{}) (string, error) {
	// Validate template for forbidden directives that could be exploited
	// Block: call (function calls), define (template definition), template (template inclusion)
	forbiddenDirectives := []string{"{{call", "{{define", "{{template", "{{block"}
	for _, directive := range forbiddenDirectives {
		if strings.Contains(tmpl, directive) {
			return "", fmt.Errorf("template contains forbidden directive: %s", directive)
		}
	}

	// Parse with strict options
	t, err := template.New("prompt").
		Option("missingkey=error"). // Fail on missing keys to prevent silent errors
		Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// TruncateString truncates a string to maxLen runes (Unicode-safe)
// Uses runes instead of bytes to properly handle multi-byte UTF-8 characters
func TruncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
