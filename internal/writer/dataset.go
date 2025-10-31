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
	file    *os.File
	mu      sync.Mutex
	logger  *slog.Logger
	records []models.DatasetRecord // In-memory buffer for async judge updates
}

// NewDatasetWriter creates a new dataset writer
// expectedRecords should be NumSubtopics × NumPromptsPerSubtopic for optimal pre-allocation
func NewDatasetWriter(sessionMgr *SessionManager, logger *slog.Logger, resumeMode bool, expectedRecords int) (*DatasetWriter, error) {
	datasetPath := sessionMgr.GetDatasetPath()

	var file *os.File
	var err error

	if resumeMode {
		// Append mode: continue existing file
		file, err = os.OpenFile(datasetPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open dataset file for append: %w", err)
		}
		logger.Info("Opened dataset file for append", "path", datasetPath)
	} else {
		// Create mode: new file
		file, err = os.Create(datasetPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create dataset file: %w", err)
		}
		logger.Info("Created dataset file", "path", datasetPath)
	}

	// Pre-allocate based on expected record count for optimal performance
	// Falls back to 1024 if expectedRecords is 0 or invalid
	initialCapacity := expectedRecords
	if initialCapacity <= 0 {
		initialCapacity = 1024
	}

	return &DatasetWriter{
		file:    file,
		logger:  logger,
		records: make([]models.DatasetRecord, 0, initialCapacity),
	}, nil
}

// WriteRecord writes a single record to the in-memory buffer and returns its index
// The record will be written to file during Close()
func (dw *DatasetWriter) WriteRecord(record models.DatasetRecord) (int, error) {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	// Add to in-memory buffer
	index := len(dw.records)
	dw.records = append(dw.records, record)

	return index, nil
}

// UpdateRecord updates a previously written record with judge results
func (dw *DatasetWriter) UpdateRecord(index int, judgeResult *models.JudgeResult) error {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	if index < 0 || index >= len(dw.records) {
		return fmt.Errorf("invalid record index: %d", index)
	}

	// Update judge fields in memory
	dw.records[index].ChosenScores = judgeResult.ChosenScores
	dw.records[index].RejectedScores = judgeResult.RejectedScores
	dw.records[index].ChosenScoreTotal = judgeResult.ChosenScoreTotal
	dw.records[index].RejectedScoreTotal = judgeResult.RejectedScoreTotal
	dw.records[index].PreferenceMargin = judgeResult.PreferenceMargin

	return nil
}

// Flush writes all buffered records to disk
func (dw *DatasetWriter) Flush() error {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	dw.logger.Info("Flushing records to disk", "count", len(dw.records))

	for i, record := range dw.records {
		data, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("failed to marshal record %d: %w", i, err)
		}

		if _, err := dw.file.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("failed to write record %d: %w", i, err)
		}
	}

	dw.logger.Info("Successfully flushed all records")
	return nil
}

// Close flushes all records and closes the dataset file
func (dw *DatasetWriter) Close() error {
	// Flush all buffered records to disk
	if err := dw.Flush(); err != nil {
		return fmt.Errorf("failed to flush records: %w", err)
	}

	if err := dw.file.Sync(); err != nil {
		dw.logger.Warn("Failed to sync dataset file", "error", err)
	}

	if err := dw.file.Close(); err != nil {
		return fmt.Errorf("failed to close dataset file: %w", err)
	}

	dw.logger.Info("Closed dataset file")
	return nil
}
