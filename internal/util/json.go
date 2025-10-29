package util

import (
	"regexp"
	"strings"
)

// Precompiled regex patterns for performance (compiled once at package init)
var (
	jsonCodeBlockRegex = regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)```")
)

// ExtractJSON extracts JSON content from a response that may contain markdown code blocks
// and attempts to fix truncated JSON arrays and objects
// Handles both arrays [] and objects {}
func ExtractJSON(s string) string {
	// Try to extract from markdown code blocks using precompiled regex
	matches := jsonCodeBlockRegex.FindStringSubmatch(s)
	if len(matches) > 1 {
		s = strings.TrimSpace(matches[1])
	} else {
		s = strings.TrimSpace(s)
	}

	// Try to find JSON array boundaries first
	arrayStart := strings.Index(s, "[")
	if arrayStart != -1 {
		arrayEnd := findMatchingBracket(s, arrayStart, '[', ']')
		if arrayEnd != -1 {
			// Found complete array
			return s[arrayStart : arrayEnd+1]
		}
		// Truncated array - try to close it
		lastQuote := strings.LastIndex(s, "\"")
		if lastQuote > arrayStart {
			// Has content, close the array
			trimmed := strings.TrimRight(s[arrayStart:], " \n\t,")
			return trimmed + "]"
		}
	}

	// Try to find JSON object boundaries
	objectStart := strings.Index(s, "{")
	if objectStart != -1 {
		objectEnd := findMatchingBracket(s, objectStart, '{', '}')
		if objectEnd != -1 {
			return s[objectStart : objectEnd+1]
		}
	}

	// Return as-is if no extraction needed
	return s
}

// findMatchingBracket finds the matching closing bracket for an opening bracket
// using proper bracket matching that handles escaped quotes and strings
// Returns -1 if no matching bracket is found
func findMatchingBracket(s string, startPos int, openChar, closeChar rune) int {
	count := 0
	inString := false
	escaped := false

	for i := startPos; i < len(s); i++ {
		ch := rune(s[i])

		// Handle escape sequences
		if escaped {
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		// Handle strings
		if ch == '"' {
			inString = !inString
			continue
		}

		// Only count brackets outside of strings
		if !inString {
			if ch == openChar {
				count++
			} else if ch == closeChar {
				count--
				if count == 0 {
					return i
				}
			}
		}
	}

	return -1 // No matching bracket found
}

// SanitizeJSON fixes common JSON issues from LLM responses
// Specifically handles:
// - Unescaped newlines in string values
// - Single quotes instead of double quotes for property values
func SanitizeJSON(s string) string {
	// First pass: Fix single-quoted values (e.g., "key": 'value with "quotes"')
	// This pattern matches: "key": 'value...' and converts to: "key": "value..."
	singleQuotePattern := regexp.MustCompile(`"([^"]+)":\s*'([^']*)'`)
	s = singleQuotePattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract the key and value
		parts := singleQuotePattern.FindStringSubmatch(match)
		if len(parts) == 3 {
			key := parts[1]
			value := parts[2]
			// Escape any double quotes in the value
			value = strings.ReplaceAll(value, `"`, `\"`)
			return `"` + key + `": "` + value + `"`
		}
		return match
	})

	// Second pass: Handle unescaped newlines and other issues
	var result strings.Builder
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escaped {
			result.WriteByte(ch)
			escaped = false
			continue
		}

		if ch == '\\' {
			result.WriteByte(ch)
			escaped = true
			continue
		}

		if ch == '"' {
			result.WriteByte(ch)
			inString = !inString
			continue
		}

		// Replace literal newlines in strings with \n
		if inString && (ch == '\n' || ch == '\r') {
			result.WriteString("\\n")
			// Skip \r if followed by \n
			if ch == '\r' && i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			continue
		}

		result.WriteByte(ch)
	}

	return result.String()
}

// RepairJSON attempts to fix common JSON issues from LLM responses
// Handles: trailing commas, missing commas, truncated arrays, empty elements
func RepairJSON(s string) string {
	// First extract JSON to handle truncation
	s = ExtractJSON(s)

	// Then sanitize newlines and escape characters
	s = SanitizeJSON(s)

	var result strings.Builder
	inString := false
	escaped := false
	lastNonWhitespace := byte(0)

	for i := 0; i < len(s); i++ {
		ch := s[i]

		// Track escape sequences
		if escaped {
			result.WriteByte(ch)
			escaped = false
			lastNonWhitespace = ch
			continue
		}

		if ch == '\\' {
			result.WriteByte(ch)
			escaped = true
			continue
		}

		// Track string boundaries
		if ch == '"' {
			// Fix missing comma before string: check if we need a comma
			if !inString && (lastNonWhitespace == '"' || lastNonWhitespace == '}' || lastNonWhitespace == ']') {
				result.WriteByte(',')
			}
			inString = !inString
			result.WriteByte(ch)
			lastNonWhitespace = ch
			continue
		}

		// Only fix issues outside of strings
		if !inString {
			// Fix trailing commas: ,] or ,}
			if ch == ']' || ch == '}' {
				// Remove trailing comma if present
				if lastNonWhitespace == ',' {
					// Remove the comma by reconstructing without it
					resultStr := result.String()
					if len(resultStr) > 0 {
						// Find and remove the last comma
						for j := len(resultStr) - 1; j >= 0; j-- {
							if resultStr[j] == ',' {
								result.Reset()
								result.WriteString(resultStr[:j])
								result.WriteString(resultStr[j+1:])
								break
							}
						}
					}
				}
				result.WriteByte(ch)
				lastNonWhitespace = ch
				continue
			}

			// Fix consecutive commas: ,, -> ,
			if ch == ',' && lastNonWhitespace == ',' {
				// Skip this comma
				continue
			}

			// Fix missing comma before object or array: "}[" or "}{"
			if (ch == '[' || ch == '{') && (lastNonWhitespace == '}' || lastNonWhitespace == ']' || lastNonWhitespace == '"') {
				result.WriteByte(',')
				result.WriteByte(ch)
				lastNonWhitespace = ch
				continue
			}

			// Track non-whitespace characters
			if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' {
				lastNonWhitespace = ch
			}
		}

		result.WriteByte(ch)
	}

	return result.String()
}
