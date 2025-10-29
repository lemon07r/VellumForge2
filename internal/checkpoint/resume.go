package checkpoint

import (
	"fmt"

	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/pkg/models"
)

// ValidateCheckpoint verifies checkpoint is compatible with current config
func ValidateCheckpoint(cp *models.Checkpoint, cfg *config.Config) error {
	expectedHash := computeConfigHash(cfg)
	if cp.ConfigHash != expectedHash {
		return fmt.Errorf("checkpoint config mismatch: checkpoint was created with different topic/counts (hash: %s vs %s)", cp.ConfigHash, expectedHash)
	}

	// Additional validation
	if cp.CurrentPhase == models.PhaseComplete {
		return fmt.Errorf("checkpoint is already complete, nothing to resume")
	}

	return nil
}

// GetPendingJobs returns jobs that still need processing
func GetPendingJobs(cp *models.Checkpoint) []models.GenerationJob {
	if !cp.PromptsComplete {
		return nil // Need to complete prompts phase first
	}

	var pending []models.GenerationJob
	for _, job := range cp.Prompts {
		if !cp.CompletedJobIDs[job.ID] {
			pending = append(pending, job)
		}
	}
	return pending
}

// GetCompletedCount returns the number of completed jobs
func GetCompletedCount(cp *models.Checkpoint) int {
	return len(cp.CompletedJobIDs)
}

// GetTotalCount returns the total number of jobs
func GetTotalCount(cp *models.Checkpoint) int {
	return len(cp.Prompts)
}

// GetProgressPercentage returns completion percentage
func GetProgressPercentage(cp *models.Checkpoint) float64 {
	total := GetTotalCount(cp)
	if total == 0 {
		return 0.0
	}
	completed := GetCompletedCount(cp)
	return float64(completed) / float64(total) * 100.0
}
