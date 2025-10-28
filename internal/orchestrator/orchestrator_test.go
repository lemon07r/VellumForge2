package orchestrator

import (
	"strings"
	"testing"
)

func TestValidateJSONArray(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantValid   bool
		wantCount   int
		wantErrNil  bool
	}{
		{
			name:       "valid array",
			input:      `["a", "b", "c"]`,
			wantValid:  true,
			wantCount:  3,
			wantErrNil: true,
		},
		{
			name:       "empty array",
			input:      `[]`,
			wantValid:  true,
			wantCount:  0,
			wantErrNil: true,
		},
		{
			name:       "array with whitespace",
			input:      `  [ "a" , "b" ]  `,
			wantValid:  true,
			wantCount:  2,
			wantErrNil: true,
		},
		{
			name:       "invalid - not array",
			input:      `{"key": "value"}`,
			wantValid:  false,
			wantCount:  0,
			wantErrNil: false,
		},
		{
			name:       "invalid - trailing comma (should fail json.Valid)",
			input:      `["a", "b",]`,
			wantValid:  false,
			wantCount:  0,
			wantErrNil: false,
		},
		{
			name:       "invalid - missing closing bracket",
			input:      `["a", "b"`,
			wantValid:  false,
			wantCount:  0,
			wantErrNil: false,
		},
		{
			name:       "invalid - not JSON",
			input:      `just some text`,
			wantValid:  false,
			wantCount:  0,
			wantErrNil: false,
		},
		{
			name:       "valid array with numbers",
			input:      `[1, 2, 3]`,
			wantValid:  true,
			wantCount:  3,
			wantErrNil: true,
		},
		{
			name:       "valid nested array",
			input:      `[["a"], ["b"]]`,
			wantValid:  true,
			wantCount:  2,
			wantErrNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, count, err := ValidateJSONArray(tt.input)

			if valid != tt.wantValid {
				t.Errorf("ValidateJSONArray() valid = %v, want %v", valid, tt.wantValid)
			}
			if count != tt.wantCount {
				t.Errorf("ValidateJSONArray() count = %v, want %v", count, tt.wantCount)
			}
			if (err == nil) != tt.wantErrNil {
				t.Errorf("ValidateJSONArray() error = %v, wantErrNil %v", err, tt.wantErrNil)
			}
		})
	}
}

func TestValidateStringArray(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedMin int
		wantCount   int
		wantErr     bool
	}{
		{
			name:        "sufficient count",
			input:       `["a", "b", "c"]`,
			expectedMin: 2,
			wantCount:   3,
			wantErr:     false,
		},
		{
			name:        "exact count",
			input:       `["a", "b"]`,
			expectedMin: 2,
			wantCount:   2,
			wantErr:     false,
		},
		{
			name:        "insufficient count",
			input:       `["a"]`,
			expectedMin: 5,
			wantCount:   1,
			wantErr:     true,
		},
		{
			name:        "filters empty strings",
			input:       `["a", "", "b", "  ", "c"]`,
			expectedMin: 2,
			wantCount:   3,
			wantErr:     false,
		},
		{
			name:        "trims whitespace",
			input:       `[" a ", "  b  ", "c"]`,
			expectedMin: 2,
			wantCount:   3,
			wantErr:     false,
		},
		{
			name:        "empty array with min 0",
			input:       `[]`,
			expectedMin: 0,
			wantCount:   0,
			wantErr:     false,
		},
		{
			name:        "all empty strings filtered",
			input:       `["", "  ", "   "]`,
			expectedMin: 1,
			wantCount:   0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, count, err := ValidateStringArray(tt.input, tt.expectedMin)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStringArray() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && count != tt.wantCount {
				t.Errorf("ValidateStringArray() count = %v, want %v", count, tt.wantCount)
			}
			if !tt.wantErr && len(items) != tt.wantCount {
				t.Errorf("ValidateStringArray() items len = %v, want %v", len(items), tt.wantCount)
			}
		})
	}
}

func TestDeduplicateStrings(t *testing.T) {
	tests := []struct {
		name      string
		input     []string
		wantCount int
		wantFirst string // Check first element to verify order preservation
	}{
		{
			name:      "no duplicates",
			input:     []string{"a", "b", "c"},
			wantCount: 3,
			wantFirst: "a",
		},
		{
			name:      "with duplicates",
			input:     []string{"a", "b", "a", "c"},
			wantCount: 3,
			wantFirst: "a",
		},
		{
			name:      "case insensitive duplicates",
			input:     []string{"Apple", "banana", "APPLE", "Banana"},
			wantCount: 2,
			wantFirst: "Apple",
		},
		{
			name:      "preserve original casing",
			input:     []string{"Apple", "BANANA", "Cherry"},
			wantCount: 3,
			wantFirst: "Apple",
		},
		{
			name:      "with whitespace variations",
			input:     []string{" a ", "b", "a", " b "},
			wantCount: 2,
			wantFirst: "a",
		},
		{
			name:      "empty strings filtered",
			input:     []string{"a", "", "b", "  ", "c"},
			wantCount: 3,
			wantFirst: "a",
		},
		{
			name:      "all empty",
			input:     []string{"", "  ", "   "},
			wantCount: 0,
			wantFirst: "",
		},
		{
			name:      "empty input",
			input:     []string{},
			wantCount: 0,
			wantFirst: "",
		},
		{
			name:      "single item",
			input:     []string{"only one"},
			wantCount: 1,
			wantFirst: "only one",
		},
		{
			name:      "all duplicates",
			input:     []string{"same", "same", "same"},
			wantCount: 1,
			wantFirst: "same",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deduplicateStrings(tt.input)

			if len(got) != tt.wantCount {
				t.Errorf("deduplicateStrings() len = %v, want %v", len(got), tt.wantCount)
			}

			// Check first element if we expect results
			if tt.wantCount > 0 && len(got) > 0 {
				if got[0] != tt.wantFirst {
					t.Errorf("deduplicateStrings() first element = %v, want %v", got[0], tt.wantFirst)
				}
			}

			// Verify all elements are unique (case-insensitive)
			seen := make(map[string]bool)
			for _, item := range got {
				normalized := strings.ToLower(strings.TrimSpace(item))
				if seen[normalized] {
					t.Errorf("deduplicateStrings() contains duplicate: %v", item)
				}
				seen[normalized] = true
			}
		})
	}
}
