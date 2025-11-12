package writer

import "github.com/lamim/vellumforge2/pkg/models"

// Writer is the interface for dataset writers
// Supports both single dataset and dual dataset (with/without reasoning) modes
type Writer interface {
	// WriteSFTRecord writes an SFT record
	// reasoning parameter is used only in dual dataset mode
	WriteSFTRecord(record models.SFTRecord, reasoning string) error

	// WriteDPORecord writes a DPO record
	// chosenReasoning and rejectedReasoning are used only in dual dataset mode
	WriteDPORecord(record models.DPORecord, chosenReasoning, rejectedReasoning string) error

	// WriteKTORecord writes a KTO record
	// reasoning parameter is used only in dual dataset mode
	WriteKTORecord(record models.KTORecord, reasoning string) error

	// WriteRecord writes a buffered record (for MO-DPO mode with async judge)
	// Returns the record index for later updates
	WriteRecord(record models.DatasetRecord) (int, error)

	// UpdateRecord updates a record with judge results
	UpdateRecord(recordIndex int, judgeResult *models.JudgeResult) error

	// Flush writes all buffered records to disk
	Flush() error

	// Close closes the writer
	Close() error
}
