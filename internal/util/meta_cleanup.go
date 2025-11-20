package util

import "strings"

// CleanMetaFromLLMResponse trims obvious meta or self-referential chatter
// (e.g. "We don't want too many lines...", "Sure! Let's start over")
// from the end of an LLM response while preserving the main story text.
func CleanMetaFromLLMResponse(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}

	lower := strings.ToLower(trimmed)
	cutIndex := len(trimmed)
	phrases := []string{
		"we don't want too many lines",
		"sure! let's start over",
		"let's start over",
		"i realize i wrote a confusing mixture of flows",
		"i realize i wrote a confusing mixture",
		"i'll rewrite and incorporate all",
	}

	for _, phrase := range phrases {
		p := strings.ToLower(phrase)
		if idx := strings.Index(lower, p); idx >= 0 && idx < cutIndex {
			cutIndex = idx
		}
	}

	if cutIndex < len(trimmed) {
		result := strings.TrimSpace(trimmed[:cutIndex])
		if result != "" {
			return result
		}
	}

	return trimmed
}
