package util

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"text/template"
)

// Template cache for performance (thread-safe)
var (
	templateCache sync.Map // map[string]*template.Template
)

// RenderTemplate renders a template string with the given data
// Templates are cached for performance (thread-safe with sync.Map)
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

	// Check cache first (O(1) lookup with sync.Map)
	if cached, ok := templateCache.Load(tmpl); ok {
		t := cached.(*template.Template)
		var buf bytes.Buffer
		if err := t.Execute(&buf, data); err != nil {
			return "", fmt.Errorf("failed to execute template: %w", err)
		}
		return buf.String(), nil
	}

	// Parse template (cache miss)
	t, err := template.New("prompt").
		Option("missingkey=error"). // Fail on missing keys to prevent silent errors
		Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// Store in cache for next time
	templateCache.Store(tmpl, t)

	// Execute
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// ClearTemplateCache clears the template cache (useful for testing)
func ClearTemplateCache() {
	templateCache = sync.Map{}
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
