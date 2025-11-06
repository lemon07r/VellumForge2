package orchestrator

import (
	"strings"
)

// Common refusal patterns from LLM responses
var refusalPatterns = []string{
	"i'm sorry, but i can't help with that",
	"i cannot help with that",
	"i can't assist with that",
	"i'm unable to help with that",
	"i apologize, but i cannot",
	"i'm not able to assist",
	"i cannot provide",
	"i cannot generate",
	"i'm sorry, i cannot",
	"i'm sorry, but i cannot",
	"as an ai",
	"i don't feel comfortable",
}

// isRefusalResponse checks if a response contains refusal patterns
func isRefusalResponse(text string) bool {
	if len(strings.TrimSpace(text)) < 50 {
		return true // Too short, likely a refusal
	}

	textLower := strings.ToLower(text)
	for _, pattern := range refusalPatterns {
		if strings.Contains(textLower, pattern) {
			return true
		}
	}
	return false
}

// getRefusalReason returns a description of why the response was considered a refusal
func getRefusalReason(text string) string {
	if len(strings.TrimSpace(text)) < 50 {
		return "response too short (< 50 chars)"
	}

	textLower := strings.ToLower(text)
	for _, pattern := range refusalPatterns {
		if strings.Contains(textLower, pattern) {
			return "contains refusal pattern: " + pattern
		}
	}
	return "unknown refusal"
}
