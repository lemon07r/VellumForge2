package checkpoint

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/pkg/models"
)

func TestNewManager(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		Generation: config.GenerationConfig{
			MainTopic:             "Test Topic",
			NumSubtopics:          10,
			NumPromptsPerSubtopic: 5,
			EnableCheckpointing:   true,
			CheckpointInterval:    10,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManager(tempDir, cfg, logger)

	if mgr == nil {
		t.Fatal("NewManager returned nil")
		return
	}

	if mgr.sessionDir != tempDir {
		t.Errorf("Expected sessionDir %s, got %s", tempDir, mgr.sessionDir)
	}

	if mgr.interval != 10 {
		t.Errorf("Expected interval 10, got %d", mgr.interval)
	}

	if !mgr.enabled {
		t.Error("Expected enabled to be true")
	}

	// Clean up
	if err := mgr.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		Generation: config.GenerationConfig{
			MainTopic:             "Test Topic",
			NumSubtopics:          10,
			NumPromptsPerSubtopic: 5,
			EnableCheckpointing:   true,
			CheckpointInterval:    1,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManager(tempDir, cfg, logger)
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Errorf("Close() failed: %v", err)
		}
	}()

	// Mark subtopics complete
	subtopics := []string{"topic1", "topic2", "topic3"}
	if err := mgr.MarkSubtopicsComplete(subtopics); err != nil {
		t.Fatalf("MarkSubtopicsComplete failed: %v", err)
	}

	// Wait a bit for async write
	time.Sleep(100 * time.Millisecond)

	// Load checkpoint
	loaded, err := Load(tempDir, logger)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !loaded.SubtopicsComplete {
		t.Error("Expected SubtopicsComplete to be true")
	}

	if len(loaded.Subtopics) != 3 {
		t.Errorf("Expected 3 subtopics, got %d", len(loaded.Subtopics))
	}

	if loaded.CurrentPhase != models.PhasePrompts {
		t.Errorf("Expected phase %s, got %s", models.PhasePrompts, loaded.CurrentPhase)
	}
}

func TestMarkJobComplete(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		Generation: config.GenerationConfig{
			MainTopic:             "Test Topic",
			NumSubtopics:          10,
			NumPromptsPerSubtopic: 5,
			EnableCheckpointing:   true,
			CheckpointInterval:    2,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManager(tempDir, cfg, logger)
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Errorf("Close() failed: %v", err)
		}
	}()

	stats := &models.SessionStats{
		TotalPrompts: 10,
		SuccessCount: 1,
		FailureCount: 0,
	}

	// Mark first job
	if err := mgr.MarkJobComplete(1, stats); err != nil {
		t.Fatalf("MarkJobComplete(1) failed: %v", err)
	}

	cp := mgr.GetCheckpoint()
	if !cp.CompletedJobIDs[1] {
		t.Error("Expected job 1 to be marked complete")
	}

	// Mark second job - should trigger save
	stats.SuccessCount = 2
	if err := mgr.MarkJobComplete(2, stats); err != nil {
		t.Fatalf("MarkJobComplete(2) failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Load and verify
	loaded, err := Load(tempDir, logger)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !loaded.CompletedJobIDs[1] || !loaded.CompletedJobIDs[2] {
		t.Error("Expected jobs 1 and 2 to be saved")
	}

	if loaded.Stats.SuccessCount != 2 {
		t.Errorf("Expected SuccessCount 2, got %d", loaded.Stats.SuccessCount)
	}
}

func TestAsyncWriteBuffer(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		Generation: config.GenerationConfig{
			MainTopic:             "Test Topic",
			NumSubtopics:          10,
			NumPromptsPerSubtopic: 5,
			EnableCheckpointing:   true,
			CheckpointInterval:    5, // Save every 5 jobs (more realistic)
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManager(tempDir, cfg, logger)

	// Mark many jobs quickly to test async buffer
	stats := &models.SessionStats{
		TotalPrompts: 25,
	}

	// Mark 25 jobs - should trigger 5 checkpoint saves (at jobs 5, 10, 15, 20, 25)
	for i := 1; i <= 25; i++ {
		stats.SuccessCount = i
		if err := mgr.MarkJobComplete(i, stats); err != nil {
			t.Fatalf("MarkJobComplete(%d) failed: %v", i, err)
		}
	}

	// Close manager to flush all pending writes
	if err := mgr.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Load and verify final checkpoint has all jobs
	// Note: Due to async nature, intermediate checkpoints may be overwritten
	// but the final state should have all jobs
	loaded, err := Load(tempDir, logger)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// The final checkpoint should have at least the jobs that triggered saves
	// In practice, it should have all 25, but we verify at least completeness
	if len(loaded.CompletedJobIDs) < 25 {
		t.Errorf("Expected at least 25 completed jobs, got %d", len(loaded.CompletedJobIDs))
	}

	// Verify all jobs are marked complete
	for i := 1; i <= 25; i++ {
		if !loaded.CompletedJobIDs[i] {
			t.Errorf("Expected job %d to be saved", i)
		}
	}

	// Verify statistics
	if loaded.Stats.SuccessCount != 25 {
		t.Errorf("Expected SuccessCount 25, got %d", loaded.Stats.SuccessCount)
	}
}

func TestCheckpointNotEnabledNoFiles(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		Generation: config.GenerationConfig{
			MainTopic:             "Test Topic",
			NumSubtopics:          10,
			NumPromptsPerSubtopic: 5,
			EnableCheckpointing:   false, // Disabled
			CheckpointInterval:    10,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManager(tempDir, cfg, logger)
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Errorf("Close() failed: %v", err)
		}
	}()

	// Try to save
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save() should not error when disabled: %v", err)
	}

	// Verify no checkpoint file was created
	checkpointPath := filepath.Join(tempDir, CheckpointFilename)
	if _, err := os.Stat(checkpointPath); !os.IsNotExist(err) {
		t.Error("Checkpoint file should not exist when checkpointing is disabled")
	}
}

func TestConfigHashValidation(t *testing.T) {
	cfg1 := &config.Config{
		Generation: config.GenerationConfig{
			MainTopic:             "Topic1",
			NumSubtopics:          10,
			NumPromptsPerSubtopic: 5,
		},
	}

	cfg2 := &config.Config{
		Generation: config.GenerationConfig{
			MainTopic:             "Topic2", // Different!
			NumSubtopics:          10,
			NumPromptsPerSubtopic: 5,
		},
	}

	hash1 := computeConfigHash(cfg1)
	hash2 := computeConfigHash(cfg2)

	if hash1 == hash2 {
		t.Error("Different configs should produce different hashes")
	}

	// Same config should produce same hash
	hash1b := computeConfigHash(cfg1)
	if hash1 != hash1b {
		t.Error("Same config should produce same hash")
	}
}
