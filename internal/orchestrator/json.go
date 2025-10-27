package orchestrator

import (
	"regexp"
	"strings"
)

// extractJSON extracts JSON content from a response that may contain markdown code blocks
// and attempts to fix truncated JSON arrays
func extractJSON(s string) string {
	// Try to extract from markdown code blocks
	re := regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)```")
	matches := re.FindStringSubmatch(s)
	if len(matches) > 1 {
		s = strings.TrimSpace(matches[1])
	} else {
		s = strings.TrimSpace(s)
	}

	// Try to find JSON array boundaries
	arrayStart := strings.Index(s, "[")

	if arrayStart != -1 {
		// Find the matching closing bracket by counting brackets
		bracketCount := 0
		inString := false
		escaped := false
		arrayEnd := -1

		for i := arrayStart; i < len(s); i++ {
			ch := s[i]

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
				if ch == '[' {
					bracketCount++
				} else if ch == ']' {
					bracketCount--
					if bracketCount == 0 {
						arrayEnd = i
						break
					}
				}
			}
		}

		if arrayEnd != -1 {
			// Found complete array
			return s[arrayStart : arrayEnd+1]
		} else {
			// Truncated array - try to close it
			lastQuote := strings.LastIndex(s, "\"")
			if lastQuote > arrayStart {
				// Has content, close the array
				trimmed := strings.TrimRight(s[arrayStart:], " \n\t,")
				return trimmed + "]"
			}
		}
	}

	// Try to find JSON object boundaries
	objectStart := strings.Index(s, "{")
	if objectStart != -1 {
		// Find the matching closing brace by counting braces
		braceCount := 0
		inString := false
		escaped := false
		objectEnd := -1

		for i := objectStart; i < len(s); i++ {
			ch := s[i]

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

			// Only count braces outside of strings
			if !inString {
				if ch == '{' {
					braceCount++
				} else if ch == '}' {
					braceCount--
					if braceCount == 0 {
						objectEnd = i
						break
					}
				}
			}
		}

		if objectEnd != -1 {
			return s[objectStart : objectEnd+1]
		}
	}

	// Return as-is if no extraction needed
	return s
}
