package util

import (
	"regexp"
	"strings"
)

// Precompiled regex patterns for think tag detection and extraction
var (
	// Matches various think/reasoning tag formats
	thinkTagRegex = regexp.MustCompile(`(?i)<think(?:ing)?>([\s\S]*?)</think(?:ing)?>`)
	// Matches Chinese reasoning tags (some Chinese models use these)
	chineseThinkTagRegex = regexp.MustCompile(`(?i)<思考>([\s\S]*?)</思考>`)
)

// ContainsThinkTags checks if the response contains think/reasoning tags
func ContainsThinkTags(response string) bool {
	return thinkTagRegex.MatchString(response) || chineseThinkTagRegex.MatchString(response)
}

// ExtractThinkContent extracts only the content within think/reasoning tags
// Returns empty string if no think tags found
func ExtractThinkContent(response string) string {
	var thinkContent []string

	// Extract English think tags
	matches := thinkTagRegex.FindAllStringSubmatch(response, -1)
	for _, match := range matches {
		if len(match) > 1 {
			thinkContent = append(thinkContent, strings.TrimSpace(match[1]))
		}
	}

	// Extract Chinese think tags
	chineseMatches := chineseThinkTagRegex.FindAllStringSubmatch(response, -1)
	for _, match := range chineseMatches {
		if len(match) > 1 {
			thinkContent = append(thinkContent, strings.TrimSpace(match[1]))
		}
	}

	return strings.Join(thinkContent, "\n\n")
}

// StripThinkTags removes think/reasoning tags and their content from response
// This gives you the final answer without the reasoning process
func StripThinkTags(response string) string {
	// Remove English think tags
	result := thinkTagRegex.ReplaceAllString(response, "")
	// Remove Chinese think tags
	result = chineseThinkTagRegex.ReplaceAllString(result, "")
	// Clean up extra whitespace
	return strings.TrimSpace(result)
}

// SplitThinkAndAnswer splits response into thinking content and final answer
// Returns (thinkContent, answer)
func SplitThinkAndAnswer(response string) (string, string) {
	thinkContent := ExtractThinkContent(response)
	answer := StripThinkTags(response)
	return thinkContent, answer
}
