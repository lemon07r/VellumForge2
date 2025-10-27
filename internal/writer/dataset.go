package writer

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/lamim/vellumforge2/pkg/models"
)

// DatasetWriter handles thread-safe writing to the dataset file
type DatasetWriter struct {
	file   *os.File
	mu     sync.Mutex
	logger *slog.Logger
}

// NewDatasetWriter creates a new dataset writer
func NewDatasetWriter(sessionMgr *SessionManager, logger *slog.Logger) (*DatasetWriter, error) {
	datasetPath := sessionMgr.GetDatasetPath()

	file, err := os.Create(datasetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create dataset file: %w", err)
	}

	logger.Info("Created dataset file", "path", datasetPath)

	return &DatasetWriter{
		file:   file,
		logger: logger,
	}, nil
}

// WriteRecord writes a single record to the dataset file
func (dw *DatasetWriter) WriteRecord(record models.DatasetRecord) error {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	// Marshal to JSON
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	// Write line
	if _, err := dw.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write record: %w", err)
	}

	return nil
}

// Close closes the dataset file
func (dw *DatasetWriter) Close() error {
	if err := dw.file.Sync(); err != nil {
		dw.logger.Warn("Failed to sync dataset file", "error", err)
	}

	if err := dw.file.Close(); err != nil {
		return fmt.Errorf("failed to close dataset file: %w", err)
	}

	dw.logger.Info("Closed dataset file")
	return nil
}
