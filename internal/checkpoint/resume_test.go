package checkpoint

import (
	"testing"

	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/pkg/models"
)

func TestValidateCheckpoint(t *testing.T) {
	cfg := &config.Config{
		Generation: config.GenerationConfig{
			MainTopic:             "Test Topic",
			NumSubtopics:          10,
			NumPromptsPerSubtopic: 5,
		},
	}

	cp := &models.Checkpoint{
		ConfigHash:   computeConfigHash(cfg),
		CurrentPhase: models.PhasePrompts,
	}

	// Should validate successfully
	if err := ValidateCheckpoint(cp, cfg); err != nil {
		t.Errorf("ValidateCheckpoint failed: %v", err)
	}

	// Different config should fail
	differentCfg := &config.Config{
		Generation: config.GenerationConfig{
			MainTopic:             "Different Topic",
			NumSubtopics:          10,
			NumPromptsPerSubtopic: 5,
		},
	}

	if err := ValidateCheckpoint(cp, differentCfg); err == nil {
		t.Error("ValidateCheckpoint should fail with mismatched config")
	}

	// Complete checkpoint should fail
	cpComplete := &models.Checkpoint{
		ConfigHash:   computeConfigHash(cfg),
		CurrentPhase: models.PhaseComplete,
	}

	if err := ValidateCheckpoint(cpComplete, cfg); err == nil {
		t.Error("ValidateCheckpoint should fail for complete checkpoint")
	}
}

func TestGetPendingJobs(t *testing.T) {
	cp := &models.Checkpoint{
		PromptsComplete: true,
		Prompts: []models.GenerationJob{
			{ID: 1, Prompt: "prompt1"},
			{ID: 2, Prompt: "prompt2"},
			{ID: 3, Prompt: "prompt3"},
			{ID: 4, Prompt: "prompt4"},
		},
		CompletedJobIDs: map[int]bool{
			1: true,
			3: true,
		},
	}

	pending := GetPendingJobs(cp)

	if len(pending) != 2 {
		t.Fatalf("Expected 2 pending jobs, got %d", len(pending))
	}

	// Should be jobs 2 and 4
	expectedIDs := map[int]bool{2: true, 4: true}
	for _, job := range pending {
		if !expectedIDs[job.ID] {
			t.Errorf("Unexpected pending job ID: %d", job.ID)
		}
	}
}

func TestGetPendingJobsPromptsNotComplete(t *testing.T) {
	cp := &models.Checkpoint{
		PromptsComplete: false,
		Prompts: []models.GenerationJob{
			{ID: 1, Prompt: "prompt1"},
		},
	}

	pending := GetPendingJobs(cp)

	if pending != nil {
		t.Error("GetPendingJobs should return nil when prompts not complete")
	}
}

func TestGetProgressPercentage(t *testing.T) {
	tests := []struct {
		name        string
		total       int
		completed   int
		expectedPct float64
	}{
		{"0%", 100, 0, 0.0},
		{"50%", 100, 50, 50.0},
		{"100%", 100, 100, 100.0},
		{"Empty", 0, 0, 0.0},
		{"Partial", 77, 23, 29.87}, // 23/77 * 100
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := &models.Checkpoint{
				Prompts:         make([]models.GenerationJob, tt.total),
				CompletedJobIDs: make(map[int]bool),
			}

			for i := 0; i < tt.completed; i++ {
				cp.CompletedJobIDs[i] = true
			}

			pct := GetProgressPercentage(cp)
			if pct < tt.expectedPct-0.1 || pct > tt.expectedPct+0.1 {
				t.Errorf("Expected ~%.2f%%, got %.2f%%", tt.expectedPct, pct)
			}
		})
	}
}

func TestGetCompletedAndTotalCount(t *testing.T) {
	cp := &models.Checkpoint{
		Prompts: []models.GenerationJob{
			{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
		},
		CompletedJobIDs: map[int]bool{
			1: true,
			3: true,
			5: true,
		},
	}

	total := GetTotalCount(cp)
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	completed := GetCompletedCount(cp)
	if completed != 3 {
		t.Errorf("Expected completed 3, got %d", completed)
	}
}
