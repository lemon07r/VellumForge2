package writer

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Session name format: session_2025-10-30T14-30-00
var sessionNameRegex = regexp.MustCompile(`^session_\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}$`)

// ValidateSessionPath validates a session directory name to prevent path traversal attacks.
// It checks for:
//   - Path traversal attempts (..)
//   - Absolute paths
//   - Path separators (session name should be a simple directory name)
//   - Expected format (session_YYYY-MM-DDTHH-MM-SS)
//   - Path escaping the output directory
//
// This prevents CWE-22 (Improper Limitation of a Pathname to a Restricted Directory)
func ValidateSessionPath(sessionName string) error {
	// Check for empty
	if sessionName == "" {
		return fmt.Errorf("session name cannot be empty")
	}

	// Check for path traversal attempts
	if strings.Contains(sessionName, "..") {
		return fmt.Errorf("invalid session name: contains '..' (path traversal attempt)")
	}

	// Check for absolute paths
	if filepath.IsAbs(sessionName) {
		return fmt.Errorf("invalid session name: must be relative path")
	}

	// Check for path separators (session name should be simple directory name)
	if strings.ContainsAny(sessionName, "/\\") {
		return fmt.Errorf("invalid session name: must be directory name without path separators")
	}

	// Validate format (ensures only expected session directories are accessed)
	if !sessionNameRegex.MatchString(sessionName) {
		return fmt.Errorf("invalid session name format: expected 'session_YYYY-MM-DDTHH-MM-SS', got '%s'", sessionName)
	}

	// Additional check: ensure resolved path stays within output directory
	outputDir := "output"
	fullPath := filepath.Join(outputDir, sessionName)
	cleanPath := filepath.Clean(fullPath)

	// Verify it's under output directory
	absOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("failed to resolve output directory: %w", err)
	}

	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to resolve session path: %w", err)
	}

	// Ensure the path is actually within the output directory
	// Use separator suffix to prevent prefix attacks like "/var/image" matching "/var/image-user"
	if !strings.HasPrefix(absPath, absOutput+string(filepath.Separator)) {
		return fmt.Errorf("session path escapes output directory")
	}

	return nil
}
