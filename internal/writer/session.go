package writer

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// SessionManager manages session directories and files
type SessionManager struct {
	sessionDir string
	logger     *slog.Logger
}

// NewSessionManager creates a new session manager
func NewSessionManager(logger *slog.Logger, resumeFromSession string) (*SessionManager, error) {
	// Create output directory if it doesn't exist
	outputDir := "output"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	var sessionDir string
	if resumeFromSession != "" {
		// Resume mode: use existing session directory
		sessionDir = filepath.Join(outputDir, resumeFromSession)
		if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
			return nil, fmt.Errorf("session directory not found: %s", sessionDir)
		}
		logger.Info("Resuming from existing session", "path", sessionDir)
	} else {
		// New session: create timestamped directory
		timestamp := time.Now().Format("2006-01-02T15-04-05")
		sessionDir = filepath.Join(outputDir, "session_"+timestamp)

		if err := os.MkdirAll(sessionDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create session directory: %w", err)
		}

		logger.Info("Created new session directory", "path", sessionDir)
	}

	return &SessionManager{
		sessionDir: sessionDir,
		logger:     logger,
	}, nil
}

// GetSessionDir returns the session directory path
func (sm *SessionManager) GetSessionDir() string {
	return sm.sessionDir
}

// GetDatasetPath returns the full path to the dataset file
func (sm *SessionManager) GetDatasetPath() string {
	return filepath.Join(sm.sessionDir, "dataset.jsonl")
}

// GetLogPath returns the full path to the session log file
func (sm *SessionManager) GetLogPath() string {
	return filepath.Join(sm.sessionDir, "session.log")
}

// GetConfigBackupPath returns the full path to the config backup
func (sm *SessionManager) GetConfigBackupPath() string {
	return filepath.Join(sm.sessionDir, "config.toml.bak")
}

// BackupConfig copies the config file to the session directory
func (sm *SessionManager) BackupConfig(configPath string) error {
	source, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	backupPath := sm.GetConfigBackupPath()
	if err := os.WriteFile(backupPath, source, 0644); err != nil {
		return fmt.Errorf("failed to write config backup: %w", err)
	}

	sm.logger.Info("Backed up config file", "path", backupPath)
	return nil
}
