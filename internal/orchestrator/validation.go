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
// For thinking models, distinguishes between actual refusals and token exhaustion during reasoning
func isRefusalResponse(text string, hasReasoning bool, finishReason string) bool {
	if len(strings.TrimSpace(text)) < 50 {
		// If we have reasoning content and hit token limit, this is token exhaustion, not refusal
		// This case is now handled separately in worker.go before calling this function
		if hasReasoning && finishReason == "length" {
			return false // Token exhaustion already handled upstream
		}
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

// isIncompleteOutput checks if a response appears to be cut off mid-generation
// This detects streaming interruptions where partial content was returned
func isIncompleteOutput(text string, finishReason string) (bool, string) {
	trimmed := strings.TrimSpace(text)

	// Very short outputs are likely incomplete (already caught by refusal check)
	if len(trimmed) < 100 {
		return true, "output too short (< 100 chars)"
	}

	// If finish_reason is 'length', it hit token limit (not incomplete, just maxed out)
	// This is different from streaming interruptions
	if finishReason == "length" {
		return false, "" // Not incomplete, just reached max tokens
	}

	// Check if output ends with terminal punctuation
	lastChar := trimmed[len(trimmed)-1]
	if lastChar != '.' && lastChar != '!' && lastChar != '?' && lastChar != '"' && lastChar != '\'' {
		// Check if the last "word" looks incomplete (lowercase, suggesting mid-sentence)
		words := strings.Fields(trimmed)
		if len(words) > 0 {
			lastWord := words[len(words)-1]
			// Remove trailing punctuation for checking
			lastWord = strings.TrimRight(lastWord, ".,;:!?\"'")

			// If last word is longer than 2 chars and ends with lowercase letter,
			// likely a mid-sentence cutoff
			if len(lastWord) > 2 {
				lastRune := rune(lastWord[len(lastWord)-1])
				if lastRune >= 'a' && lastRune <= 'z' {
					return true, fmt.Sprintf("incomplete ending: does not end with terminal punctuation, last word '%s' suggests mid-sentence cutoff", lastWord)
				}
			}
		}

		// Ends with non-terminal punctuation but not obviously incomplete
		return true, "no terminal punctuation (.!?\"') at end"
	}

	// Output appears complete
	return false, ""
}

// validateFinishReason checks if the API response completed successfully
func validateFinishReason(finishReason string, hasReasoning bool, contentLength int) (bool, string) {
	// Empty finish_reason can happen with some API implementations (e.g., nahcrof streaming)
	// Don't fail immediately - let isIncompleteOutput() do the actual content validation
	if finishReason == "" {
		// Return true but we'll rely on isIncompleteOutput() to validate actual completion
		return true, ""
	}

	// 'stop' means normal completion
	if finishReason == "stop" {
		return true, ""
	}

	// 'length' means hit token limit - this is handled separately
	if finishReason == "length" {
		// If we have reasoning and no content, this is token exhaustion (handled upstream)
		if hasReasoning && contentLength == 0 {
			return false, "token exhaustion during reasoning phase"
		}
		// Otherwise it's just a very long output that hit the limit (acceptable)
		return true, ""
	}

	// Any other finish_reason is unexpected
	return false, fmt.Sprintf("unexpected finish_reason: %s", finishReason)
}
