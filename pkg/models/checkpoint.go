package models

import "time"

// CheckpointPhase represents the current phase of generation
type CheckpointPhase string

const (
	PhaseSubtopics CheckpointPhase = "subtopics"
	PhasePrompts   CheckpointPhase = "prompts"
	PhasePairs     CheckpointPhase = "pairs"
	PhaseComplete  CheckpointPhase = "complete"
)

// Checkpoint represents the saved state of a generation session
type Checkpoint struct {
	// Session identification
	SessionID   string    `json:"session_id"`    // UUID for this session
	CreatedAt   time.Time `json:"created_at"`    // When session started
	LastSavedAt time.Time `json:"last_saved_at"` // Last checkpoint time

	// Pipeline phase tracking
	CurrentPhase CheckpointPhase `json:"current_phase"`

	// Phase 1: Subtopics (completed = we have the full list)
	SubtopicsComplete bool     `json:"subtopics_complete"`
	Subtopics         []string `json:"subtopics"`

	// Phase 2: Prompts (completed = we have all prompts for all subtopics)
	PromptsComplete   bool            `json:"prompts_complete"`
	Prompts           []GenerationJob `json:"prompts"`             // Full job list with IDs
	FailedSubtopics   []string        `json:"failed_subtopics"`    // Subtopics that failed prompt generation
	PromptSuccessRate float64         `json:"prompt_success_rate"` // Success rate for prompt generation phase

	// Phase 3: Preference Pairs (track which jobs are done)
	CompletedJobIDs map[int]bool `json:"completed_job_ids"` // job_id -> true

	// Statistics (cumulative)
	Stats SessionStats `json:"stats"`

	// Configuration snapshot (for validation)
	ConfigHash string `json:"config_hash"` // SHA256 of config for mismatch detection
}

// JobCompletion represents a completed job for incremental checkpointing
type JobCompletion struct {
	JobID     int       `json:"job_id"`
	Timestamp time.Time `json:"timestamp"`
}
