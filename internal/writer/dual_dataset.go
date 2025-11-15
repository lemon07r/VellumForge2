package writer

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/lamim/vellumforge2/internal/util"
	"github.com/lamim/vellumforge2/pkg/models"
)

// DualDatasetWriter writes to both regular and reasoning-aware datasets simultaneously
type DualDatasetWriter struct {
	regularFile   *os.File
	reasoningFile *os.File
	mu            sync.Mutex
	logger        *slog.Logger
	records       []models.DatasetRecord // In-memory buffer for async judge updates (regular only)
}

// NewDualDatasetWriter creates a writer that outputs both regular and reasoning datasets
func NewDualDatasetWriter(sessionMgr *SessionManager, logger *slog.Logger, resumeMode bool, expectedRecords int) (*DualDatasetWriter, error) {
	regularPath := sessionMgr.GetDatasetPath()
	reasoningPath := sessionMgr.GetReasoningDatasetPath()

	// Open regular dataset file
	var regularFile *os.File
	var err error
	if resumeMode {
		regularFile, err = os.OpenFile(regularPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open regular dataset for append: %w", err)
		}
		logger.Info("Opened regular dataset for append", "path", regularPath)
	} else {
		regularFile, err = os.Create(regularPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create regular dataset: %w", err)
		}
		logger.Info("Created regular dataset", "path", regularPath)
	}

	// Open reasoning dataset file
	var reasoningFile *os.File
	if resumeMode {
		reasoningFile, err = os.OpenFile(reasoningPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			_ = regularFile.Close()
			return nil, fmt.Errorf("failed to open reasoning dataset for append: %w", err)
		}
		logger.Info("Opened reasoning dataset for append", "path", reasoningPath)
	} else {
		reasoningFile, err = os.Create(reasoningPath)
		if err != nil {
			_ = regularFile.Close()
			return nil, fmt.Errorf("failed to create reasoning dataset: %w", err)
		}
		logger.Info("Created reasoning dataset", "path", reasoningPath)
	}

	initialCapacity := expectedRecords
	if initialCapacity <= 0 {
		initialCapacity = 1024
	}

	return &DualDatasetWriter{
		regularFile:   regularFile,
		reasoningFile: reasoningFile,
		logger:        logger,
		records:       make([]models.DatasetRecord, 0, initialCapacity),
	}, nil
}

// WriteSFTRecord writes an SFT record to both datasets
// Regular: standard output
// Reasoning: output with <think> tags if reasoning content present
func (dw *DualDatasetWriter) WriteSFTRecord(record models.SFTRecord, reasoning string) error {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	// Write regular record (without reasoning)
	regularData, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal regular SFT record: %w", err)
	}

	if _, err := dw.regularFile.Write(append(regularData, '\n')); err != nil {
		return fmt.Errorf("failed to write regular SFT record: %w", err)
	}

	// Write reasoning record (with think tags if reasoning present)
	reasoningRecord := applyReasoningToSFTRecord(record, reasoning)

	reasoningData, err := json.Marshal(reasoningRecord)
	if err != nil {
		return fmt.Errorf("failed to marshal reasoning SFT record: %w", err)
	}

	if _, err := dw.reasoningFile.Write(append(reasoningData, '\n')); err != nil {
		return fmt.Errorf("failed to write reasoning SFT record: %w", err)
	}

	return nil
}

// WriteDPORecord writes a DPO record to both datasets
func (dw *DualDatasetWriter) WriteDPORecord(record models.DPORecord, chosenReasoning, rejectedReasoning string) error {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	// Write regular record (without reasoning)
	regularData, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal regular DPO record: %w", err)
	}

	if _, err := dw.regularFile.Write(append(regularData, '\n')); err != nil {
		return fmt.Errorf("failed to write regular DPO record: %w", err)
	}

	// Write reasoning record (with think tags if reasoning present)
	reasoningRecord := record
	if chosenReasoning != "" {
		reasoningRecord.Chosen = util.CombineReasoningAndContent(chosenReasoning, record.Chosen)
	}
	if rejectedReasoning != "" {
		reasoningRecord.Rejected = util.CombineReasoningAndContent(rejectedReasoning, record.Rejected)
	}

	reasoningData, err := json.Marshal(reasoningRecord)
	if err != nil {
		return fmt.Errorf("failed to marshal reasoning DPO record: %w", err)
	}

	if _, err := dw.reasoningFile.Write(append(reasoningData, '\n')); err != nil {
		return fmt.Errorf("failed to write reasoning DPO record: %w", err)
	}

	return nil
}

// WriteKTORecord writes a KTO record to both datasets
func (dw *DualDatasetWriter) WriteKTORecord(record models.KTORecord, reasoning string) error {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	// Write regular record (without reasoning)
	regularData, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal regular KTO record: %w", err)
	}

	if _, err := dw.regularFile.Write(append(regularData, '\n')); err != nil {
		return fmt.Errorf("failed to write regular KTO record: %w", err)
	}

	// Write reasoning record (with think tags if reasoning present)
	reasoningRecord := record
	if reasoning != "" {
		reasoningRecord.Completion = util.CombineReasoningAndContent(reasoning, record.Completion)
	}

	reasoningData, err := json.Marshal(reasoningRecord)
	if err != nil {
		return fmt.Errorf("failed to marshal reasoning KTO record: %w", err)
	}

	if _, err := dw.reasoningFile.Write(append(reasoningData, '\n')); err != nil {
		return fmt.Errorf("failed to write reasoning KTO record: %w", err)
	}

	return nil
}

// applyReasoningToSFTRecord returns a copy of record with reasoning merged into the answer portion
func applyReasoningToSFTRecord(record models.SFTRecord, reasoning string) models.SFTRecord {
	if reasoning == "" {
		return record
	}

	reasoningRecord := record

	if len(reasoningRecord.Conversations) > 0 {
		for i := len(reasoningRecord.Conversations) - 1; i >= 0; i-- {
			from := strings.ToLower(reasoningRecord.Conversations[i].From)
			if from == "gpt" || from == "assistant" {
				reasoningRecord.Conversations[i].Value = util.CombineReasoningAndContent(
					reasoning,
					reasoningRecord.Conversations[i].Value,
				)
				return reasoningRecord
			}
		}

		// If no assistant turn found, append one containing the reasoning+content
		reasoningRecord.Conversations = append(reasoningRecord.Conversations, models.ShareGPTMessage{
			From:  "gpt",
			Value: util.CombineReasoningAndContent(reasoning, ""),
		})
		return reasoningRecord
	}

	reasoningRecord.Output = util.CombineReasoningAndContent(reasoning, record.Output)
	return reasoningRecord
}

// WriteRecord writes a buffered record (for MO-DPO mode with async judge)
// Returns the record index for later updates
// Only written to regular dataset (reasoning dataset doesn't support MO-DPO buffering)
func (dw *DualDatasetWriter) WriteRecord(record models.DatasetRecord) (int, error) {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	index := len(dw.records)
	dw.records = append(dw.records, record)
	return index, nil
}

// UpdateRecord updates a record with judge results
// Only applies to regular dataset (reasoning dataset written immediately)
func (dw *DualDatasetWriter) UpdateRecord(recordIndex int, judgeResult *models.JudgeResult) error {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	if recordIndex < 0 || recordIndex >= len(dw.records) {
		return fmt.Errorf("invalid record index: %d (total: %d)", recordIndex, len(dw.records))
	}

	// Update judge fields in memory (same as DatasetWriter)
	dw.records[recordIndex].ChosenScores = judgeResult.ChosenScores
	dw.records[recordIndex].RejectedScores = judgeResult.RejectedScores
	dw.records[recordIndex].ChosenScoreTotal = judgeResult.ChosenScoreTotal
	dw.records[recordIndex].RejectedScoreTotal = judgeResult.RejectedScoreTotal
	dw.records[recordIndex].PreferenceMargin = judgeResult.PreferenceMargin

	return nil
}

// Flush writes all buffered records to the regular dataset file
// Reasoning dataset is written immediately, so no flush needed
func (dw *DualDatasetWriter) Flush() error {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	for _, record := range dw.records {
		data, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("failed to marshal record: %w", err)
		}

		if _, err := dw.regularFile.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("failed to write record: %w", err)
		}
	}

	dw.logger.Info("Flushed buffered records to regular dataset", "count", len(dw.records))
	dw.records = dw.records[:0] // Clear buffer
	return nil
}

// Close closes both dataset files
func (dw *DualDatasetWriter) Close() error {
	var errs []error

	if err := dw.regularFile.Close(); err != nil {
		errs = append(errs, fmt.Errorf("regular dataset: %w", err))
	}

	if err := dw.reasoningFile.Close(); err != nil {
		errs = append(errs, fmt.Errorf("reasoning dataset: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing dual datasets: %v", errs)
	}

	dw.logger.Info("Closed dual dataset files")
	return nil
}
