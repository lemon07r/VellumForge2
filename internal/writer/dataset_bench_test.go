package writer

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lamim/vellumforge2/pkg/models"
)

// BenchmarkDatasetWriter_WriteRecord benchmarks writing records
func BenchmarkDatasetWriter_WriteRecord(b *testing.B) {
	// Create temp directory
	tempDir := b.TempDir()
	sessionDir := filepath.Join(tempDir, "session_test")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		b.Fatal(err)
	}

	// Create session manager
	sessionMgr := &SessionManager{
		sessionDir: sessionDir,
	}

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create writer
	writer, err := NewDatasetWriter(sessionMgr, logger, false, 1000)
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := writer.Close(); err != nil {
			b.Fatal(err)
		}
	}()

	// Benchmark record
	record := models.DatasetRecord{
		MainTopic: "Test Topic",
		SubTopic:  "Subtopic",
		Prompt:    "Test prompt",
		Chosen:    "Chosen response",
		Rejected:  "Rejected response",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = writer.WriteRecord(record)
	}
	b.StopTimer()
}

// BenchmarkDatasetWriter_UpdateRecord benchmarks updating records
func BenchmarkDatasetWriter_UpdateRecord(b *testing.B) {
	// Create temp directory
	tempDir := b.TempDir()
	sessionDir := filepath.Join(tempDir, "session_test")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		b.Fatal(err)
	}

	// Create session manager
	sessionMgr := &SessionManager{
		sessionDir: sessionDir,
	}

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create writer and add initial records
	writer, err := NewDatasetWriter(sessionMgr, logger, false, 1000)
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := writer.Close(); err != nil {
			b.Fatal(err)
		}
	}()

	// Add some records
	record := models.DatasetRecord{
		MainTopic: "Test Topic",
		SubTopic:  "Subtopic",
		Prompt:    "Test prompt",
		Chosen:    "Chosen response",
		Rejected:  "Rejected response",
	}

	indices := make([]int, 100)
	for i := 0; i < 100; i++ {
		idx, err := writer.WriteRecord(record)
		if err != nil {
			b.Fatal(err)
		}
		indices[i] = idx
	}

	// Judge result for updates
	judgeResult := &models.JudgeResult{
		ChosenScores: map[string]models.CriteriaScore{
			"accuracy": {Score: 5, Reasoning: "Highly accurate"},
			"clarity":  {Score: 4, Reasoning: "Clear explanation"},
		},
		RejectedScores: map[string]models.CriteriaScore{
			"accuracy": {Score: 2, Reasoning: "Less accurate"},
			"clarity":  {Score: 2, Reasoning: "Unclear"},
		},
		ChosenScoreTotal:   4.5,
		RejectedScoreTotal: 2.0,
		PreferenceMargin:   2.5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := indices[i%len(indices)]
		_ = writer.UpdateRecord(idx, judgeResult)
	}
	b.StopTimer()
}

// BenchmarkDatasetWriter_Close benchmarks the close operation
func BenchmarkDatasetWriter_Close(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		// Create temp directory
		tempDir := b.TempDir()
		sessionDir := filepath.Join(tempDir, "session_test")
		if err := os.MkdirAll(sessionDir, 0755); err != nil {
			b.Fatal(err)
		}

		// Create session manager
		sessionMgr := &SessionManager{
			sessionDir: sessionDir,
		}

		// Create logger
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

		// Create writer and add records
		writer, err := NewDatasetWriter(sessionMgr, logger, false, 100)
		if err != nil {
			b.Fatal(err)
		}

		// Add 100 records
		record := models.DatasetRecord{
			MainTopic: "Test Topic",
			SubTopic:  "Subtopic",
			Prompt:    "Test prompt",
			Chosen:    "Chosen response",
			Rejected:  "Rejected response",
		}

		for j := 0; j < 100; j++ {
			_, err := writer.WriteRecord(record)
			if err != nil {
				b.Fatal(err)
			}
		}

		b.StartTimer()
		if err := writer.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
