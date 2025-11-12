package util

import "fmt"

// WrapInThinkTags wraps reasoning content in <think> tags
// This format is used for reasoning-aware training datasets
func WrapInThinkTags(reasoning string) string {
	if reasoning == "" {
		return ""
	}
	return fmt.Sprintf("<think>\n%s\n</think>", reasoning)
}

// CombineReasoningAndContent combines reasoning (in think tags) with final content
// Used for creating reasoning-aware datasets where both thinking and answer are included
func CombineReasoningAndContent(reasoning, content string) string {
	if reasoning == "" {
		return content
	}
	return WrapInThinkTags(reasoning) + "\n\n" + content
}

// StripReasoningFromCombined removes think-tagged reasoning from combined output
// Useful if you need to extract just the answer from a reasoning-aware response
func StripReasoningFromCombined(combined string) string {
	return StripThinkTags(combined)
}
