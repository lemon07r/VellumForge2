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
// Specifically handles unescaped newlines in string values
func SanitizeJSON(s string) string {
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
