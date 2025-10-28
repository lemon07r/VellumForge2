package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateJSONArray performs pre-validation on JSON string before unmarshaling
// Returns: (isValid bool, elementCount int, error)
func ValidateJSONArray(jsonStr string) (bool, int, error) {
	// Quick sanity check
	trimmed := strings.TrimSpace(jsonStr)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return false, 0, fmt.Errorf("not a JSON array: missing brackets")
	}

	// Use json.Valid for syntactic validation (fast, no allocation)
	if !json.Valid([]byte(jsonStr)) {
		return false, 0, fmt.Errorf("invalid JSON syntax")
	}

	// Count elements using decoder (more efficient than unmarshal for counting)
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &arr); err != nil {
		return false, 0, fmt.Errorf("failed to parse array: %w", err)
	}

	return true, len(arr), nil
}

// ValidateStringArray validates and unmarshals a JSON array of strings
// Returns: (strings, actualCount, error)
func ValidateStringArray(jsonStr string, expectedMin int) ([]string, int, error) {
	// Pre-validate structure and syntax
	valid, _, err := ValidateJSONArray(jsonStr)
	if !valid {
		return nil, 0, err
	}

	// Unmarshal
	var items []string
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal strings: %w", err)
	}

	// Validate all elements are non-empty strings (filter out empty/whitespace-only)
	validItems := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			validItems = append(validItems, trimmed)
		}
	}

	// Check count AFTER filtering
	actualCount := len(validItems)
	if actualCount < expectedMin {
		return nil, actualCount, fmt.Errorf("insufficient elements: got %d, expected at least %d", actualCount, expectedMin)
	}

	return validItems, actualCount, nil
}

// deduplicateStrings removes duplicates while preserving order
// Uses case-insensitive comparison for duplicate detection
func deduplicateStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	unique := make([]string, 0, len(items))

	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		normalized := strings.ToLower(trimmed)
		if normalized == "" {
			continue
		}

		if !seen[normalized] {
			seen[normalized] = true
			unique = append(unique, trimmed) // Keep original casing
		}
	}

	return unique
}
