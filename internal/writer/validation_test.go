package writer

import (
	"strings"
	"testing"
)

func TestValidateSessionPath_Valid(t *testing.T) {
	tests := []string{
		"session_2025-10-30T14-30-00",
		"session_2024-01-01T00-00-00",
		"session_2023-12-31T23-59-59",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			if err := ValidateSessionPath(tt); err != nil {
				t.Errorf("ValidateSessionPath(%q) returned unexpected error: %v", tt, err)
			}
		})
	}
}

func TestValidateSessionPath_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // substring of expected error message
	}{
		{
			name:  "empty",
			input: "",
			want:  "cannot be empty",
		},
		{
			name:  "traversal_double_dot",
			input: "../etc",
			want:  "path traversal",
		},
		{
			name:  "traversal_multiple",
			input: "../../etc/passwd",
			want:  "path traversal",
		},
		{
			name:  "traversal_in_middle",
			input: "session_2025-10-30T14-30-00/../etc",
			want:  "path traversal",
		},
		{
			name:  "absolute_unix",
			input: "/etc/passwd",
			want:  "must be relative",
		},
		{
			name:  "absolute_windows",
			input: "C:\\Windows\\System32",
			want:  "without path separators", // Caught by path separator check before absolute check
		},
		{
			name:  "with_forward_slash",
			input: "session/2025",
			want:  "without path separators",
		},
		{
			name:  "with_backslash",
			input: "session\\2025",
			want:  "without path separators",
		},
		{
			name:  "wrong_format_no_prefix",
			input: "my-session",
			want:  "invalid session name format",
		},
		// Note: Regex validates format structure, not semantic validity of dates/times
		// These pass format check but would fail on actual use - acceptable for security
		{
			name:  "wrong_format_missing_separator",
			input: "session_20251030T143000",
			want:  "invalid session name format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSessionPath(tt.input)
			if err == nil {
				t.Errorf("ValidateSessionPath(%q) expected error containing %q, got nil", tt.input, tt.want)
			} else if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ValidateSessionPath(%q) error = %v, want substring %q", tt.input, err, tt.want)
			}
		})
	}
}

// TestValidateSessionPath_AttackVectors tests various attack scenarios
func TestValidateSessionPath_AttackVectors(t *testing.T) {
	attackVectors := []struct {
		name   string
		vector string
		desc   string
	}{
		{
			name:   "classic_traversal",
			vector: "../../../etc/passwd",
			desc:   "Classic directory traversal to /etc/passwd",
		},
		{
			name:   "windows_traversal",
			vector: "..\\..\\..\\Windows\\System32",
			desc:   "Windows-style directory traversal",
		},
		{
			name:   "encoded_dots",
			vector: "session_2025-10-30T14-30-00/../secret",
			desc:   "Traversal after valid prefix",
		},
		{
			name:   "null_byte",
			vector: "session_2025-10-30T14-30-00\x00",
			desc:   "Null byte injection (should fail format check)",
		},
		{
			name:   "absolute_path_unix",
			vector: "/var/log/sensitive.log",
			desc:   "Absolute path to system file",
		},
		{
			name:   "absolute_path_windows",
			vector: "C:\\Users\\Admin\\Documents",
			desc:   "Windows absolute path",
		},
		{
			name:   "mixed_separators",
			vector: "session/2025\\10",
			desc:   "Mixed path separators",
		},
		{
			name:   "hidden_traversal",
			vector: "session_2025-10-30T14-30-00/../../",
			desc:   "Traversal with forward slashes",
		},
	}

	for _, attack := range attackVectors {
		t.Run(attack.name, func(t *testing.T) {
			err := ValidateSessionPath(attack.vector)
			if err == nil {
				t.Errorf("ValidateSessionPath(%q) should have blocked attack: %s", attack.vector, attack.desc)
			}
		})
	}
}
