package orchestrator

import (
	"fmt"
	"strings"
)

// Common refusal patterns from LLM responses
// IMPORTANT: These checks should ONLY be applied to chosen/main model responses.
// Rejected responses with these patterns are valuable training signals that teach
// the model what NOT to do. Do not filter rejected responses for refusals.
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
		return true // Too short, likely a refusal or empty response
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
		return fmt.Sprintf("response too short (< 50 chars): '%s'", strings.TrimSpace(text)[:min(40, len(strings.TrimSpace(text)))])
	}

	textLower := strings.ToLower(text)
	for _, pattern := range refusalPatterns {
		if strings.Contains(textLower, pattern) {
			return "contains refusal pattern: " + pattern
		}
	}
	return "unknown refusal"
}
