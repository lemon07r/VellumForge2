package util

import (
	"encoding/json"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType string // "array" or "object"
	}{
		{
			name:     "plain array",
			input:    `["a", "b", "c"]`,
			wantType: "array",
		},
		{
			name:     "array in markdown",
			input:    "```json\n[\"a\", \"b\", \"c\"]\n```",
			wantType: "array",
		},
		{
			name:     "truncated array",
			input:    `["a", "b", "c"`,
			wantType: "array",
		},
		{
			name:     "array with text before",
			input:    `Here are the results: ["a", "b", "c"]`,
			wantType: "array",
		},
		{
			name:     "plain object",
			input:    `{"key": "value"}`,
			wantType: "object",
		},
		{
			name:     "truncated object - missing closing brace",
			input:    `{"field1": "value1", "field2": "value2"`,
			wantType: "object",
		},
		{
			name:     "truncated object - missing nested closing braces",
			input:    `{"field1": {"score": 3}, "field2": {"score": 2}, "field3": {`,
			wantType: "object",
		},
		{
			name:     "truncated object - judge response pattern",
			input:    `{"plot": {"score": 3, "reasoning": "Good"}, "character": {"score": 2`,
			wantType: "object",
		},
		{
			name:     "object with trailing comma before truncation",
			input:    `{"field1": "value1", "field2": "value2",`,
			wantType: "object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractJSON(tt.input)

			if len(got) == 0 {
				t.Errorf("ExtractJSON() returned empty string")
				return
			}

			// Verify it's valid JSON
			if tt.wantType == "array" {
				var arr []interface{}
				if err := json.Unmarshal([]byte(got), &arr); err != nil {
					t.Errorf("ExtractJSON() produced invalid array JSON: %v\nGot: %s", err, got)
				}
			} else {
				var obj map[string]interface{}
				if err := json.Unmarshal([]byte(got), &obj); err != nil {
					t.Errorf("ExtractJSON() produced invalid object JSON: %v\nGot: %s", err, got)
				}
			}
		})
	}
}

func TestRepairJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValid bool
	}{
		{
			name:      "valid json",
			input:     `["a", "b", "c"]`,
			wantValid: true,
		},
		{
			name:      "trailing comma in array",
			input:     `["a", "b", "c",]`,
			wantValid: true,
		},
		{
			name:      "multiple trailing commas",
			input:     `["a", "b",,]`,
			wantValid: true,
		},
		{
			name:      "trailing comma with spaces",
			input:     `["a", "b", "c" , ]`,
			wantValid: true,
		},
		{
			name:      "missing comma between elements",
			input:     `["a" "b" "c"]`,
			wantValid: true,
		},
		{
			name:      "unescaped newline in string",
			input:     "[\"a\nb\"]",
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repaired := RepairJSON(tt.input)

			var arr []string
			err := json.Unmarshal([]byte(repaired), &arr)

			if tt.wantValid && err != nil {
				t.Errorf("RepairJSON() failed to produce valid JSON: %v\nInput: %s\nOutput: %s", err, tt.input, repaired)
			}
		})
	}
}

func TestSanitizeJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "unescaped newline",
			input: "[\"a\nb\"]",
			want:  "[\"a\\nb\"]",
		},
		{
			name:  "unescaped carriage return",
			input: "[\"a\rb\"]",
			want:  "[\"a\\nb\"]",
		},
		{
			name:  "valid json unchanged",
			input: `["a", "b"]`,
			want:  `["a", "b"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeJSON(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCountUnmatchedBraces(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		openChar  rune
		closeChar rune
		want      int
	}{
		{
			name:      "balanced braces",
			input:     `{"key": "value"}`,
			openChar:  '{',
			closeChar: '}',
			want:      0,
		},
		{
			name:      "one unmatched opening brace",
			input:     `{"key": "value"`,
			openChar:  '{',
			closeChar: '}',
			want:      1,
		},
		{
			name:      "two unmatched opening braces",
			input:     `{"outer": {"inner": "value"`,
			openChar:  '{',
			closeChar: '}',
			want:      2,
		},
		{
			name:      "three unmatched opening braces",
			input:     `{"a": {"b": {"c": "value"`,
			openChar:  '{',
			closeChar: '}',
			want:      3,
		},
		{
			name:      "braces in strings don't count",
			input:     `{"key": "value with { and }"`,
			openChar:  '{',
			closeChar: '}',
			want:      1,
		},
		{
			name:      "escaped quotes handled correctly",
			input:     `{"key": "value with \" quote"`,
			openChar:  '{',
			closeChar: '}',
			want:      1,
		},
		{
			name:      "array brackets",
			input:     `["a", ["b", ["c"`,
			openChar:  '[',
			closeChar: ']',
			want:      3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countUnmatchedBraces(tt.input, tt.openChar, tt.closeChar)
			if got != tt.want {
				t.Errorf("countUnmatchedBraces() = %d, want %d\nInput: %s", got, tt.want, tt.input)
			}
		})
	}
}
