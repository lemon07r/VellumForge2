package util

import (
	"testing"
)

func TestContainsThinkTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "has think tags",
			input:    "<think>Let me reason about this</think>The answer is 42",
			expected: true,
		},
		{
			name:     "has thinking tags",
			input:    "<thinking>Step by step reasoning</thinking>Final answer",
			expected: true,
		},
		{
			name:     "no think tags",
			input:    "Just a regular response without any tags",
			expected: false,
		},
		{
			name:     "has Chinese think tags",
			input:    "<思考>让我想想</思考>答案是42",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsThinkTags(tt.input)
			if result != tt.expected {
				t.Errorf("ContainsThinkTags() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractThinkContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "extract single think block",
			input:    "<think>This is my reasoning</think>The answer is 42",
			expected: "This is my reasoning",
		},
		{
			name:     "extract multiple think blocks",
			input:    "<think>First thought</think>Some text<think>Second thought</think>Answer",
			expected: "First thought\n\nSecond thought",
		},
		{
			name:     "no think tags",
			input:    "Just a regular response",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractThinkContent(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractThinkContent() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestStripThinkTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strip single think block",
			input:    "<think>This is my reasoning</think>The answer is 42",
			expected: "The answer is 42",
		},
		{
			name:     "strip multiple think blocks",
			input:    "<think>First thought</think>Some text<think>Second thought</think>Final answer",
			expected: "Some textFinal answer",
		},
		{
			name:     "no think tags",
			input:    "Just a regular response",
			expected: "Just a regular response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripThinkTags(tt.input)
			if result != tt.expected {
				t.Errorf("StripThinkTags() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSplitThinkAndAnswer(t *testing.T) {
	input := "<think>Let me solve this step by step\n1. First calculate x\n2. Then calculate y</think>The final answer is 42"
	
	thinking, answer := SplitThinkAndAnswer(input)
	
	expectedThinking := "Let me solve this step by step\n1. First calculate x\n2. Then calculate y"
	expectedAnswer := "The final answer is 42"
	
	if thinking != expectedThinking {
		t.Errorf("SplitThinkAndAnswer() thinking = %q, want %q", thinking, expectedThinking)
	}
	
	if answer != expectedAnswer {
		t.Errorf("SplitThinkAndAnswer() answer = %q, want %q", answer, expectedAnswer)
	}
}
