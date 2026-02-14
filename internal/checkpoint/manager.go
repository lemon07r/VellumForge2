package checkpoint

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/pkg/models"
)

const CheckpointFilename = "checkpoint.json"

// Manager handles checkpoint operations with async write support
type Manager struct {
	sessionDir string
	checkpoint *models.Checkpoint
	mu         sync.RWMutex
	logger     *slog.Logger
	interval   int // Save every N jobs
	jobCounter int // Counter since last save
	enabled    bool

	// Async write support
	writeChan   chan *models.Checkpoint
	writeWg     sync.WaitGroup
	stopWriter  chan struct{}
	writerError error
	errorMu     sync.Mutex
	writeMu     sync.Mutex // Protects concurrent disk writes
}

// NewManager creates a new checkpoint manager
func NewManager(sessionDir string, cfg *config.Config, logger *slog.Logger) *Manager {
	m := &Manager{
		sessionDir: sessionDir,
		checkpoint: &models.Checkpoint{
			SessionID:       uuid.New().String(),
			CreatedAt:       time.Now(),
			CurrentPhase:    models.PhaseSubtopics,
			CompletedJobIDs: make(map[int]bool),
			ConfigHash:      computeConfigHash(cfg),
		},
		logger:     logger,
		interval:   cfg.Generation.CheckpointInterval,
		enabled:    cfg.Generation.EnableCheckpointing,
		writeChan:  make(chan *models.Checkpoint, 10), // Buffer up to 10 pending writes
		stopWriter: make(chan struct{}),
	}

	if m.enabled {
		m.startAsyncWriter()
	}

	return m
}

// NewManagerFromCheckpoint creates a manager from existing checkpoint
func NewManagerFromCheckpoint(sessionDir string, cp *models.Checkpoint, cfg *config.Config, logger *slog.Logger) *Manager {
	m := &Manager{
		sessionDir: sessionDir,
		checkpoint: cp,
		logger:     logger,
		interval:   cfg.Generation.CheckpointInterval,
		enabled:    cfg.Generation.EnableCheckpointing,
		writeChan:  make(chan *models.Checkpoint, 10),
		stopWriter: make(chan struct{}),
	}

	if m.enabled {
		m.startAsyncWriter()
	}

	return m
}

// startAsyncWriter starts the background writer goroutine
func (m *Manager) startAsyncWriter() {
	m.writeWg.Add(1)
	go func() {
		defer m.writeWg.Done()
		for {
			select {
			case cp := <-m.writeChan:
				if err := m.writeCheckpointToDisk(cp); err != nil {
					m.errorMu.Lock()
					m.writerError = err
					m.errorMu.Unlock()
					m.logger.Error("Failed to write checkpoint", "error", err)
				}
			case <-m.stopWriter:
				// Drain remaining writes before stopping
				for len(m.writeChan) > 0 {
					cp := <-m.writeChan
					if err := m.writeCheckpointToDisk(cp); err != nil {
						m.logger.Error("Failed to write checkpoint during shutdown", "error", err)
					}
				}
				return
			}
		}
	}()
}

// writeCheckpointToDisk performs the actual disk write (called by async writer)
func (m *Manager) writeCheckpointToDisk(cp *models.Checkpoint) error {
	// Protect against concurrent disk writes
	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	// Marshal to JSON
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Atomic write: write to temp file, then rename
	checkpointPath := filepath.Join(m.sessionDir, CheckpointFilename)
	tempPath := checkpointPath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp checkpoint: %w", err)
	}

	if err := os.Rename(tempPath, checkpointPath); err != nil {
		return fmt.Errorf("failed to rename checkpoint: %w", err)
	}

	m.logger.Debug("Checkpoint saved", "path", checkpointPath, "phase", cp.CurrentPhase)
	return nil
}

// Save queues checkpoint for async write
func (m *Manager) Save() error {
	if !m.enabled {
		return nil
	}

	m.mu.Lock()
	m.checkpoint.LastSavedAt = time.Now()
	// Create a copy to avoid race conditions
	cpCopy := m.copyCheckpoint()
	m.mu.Unlock()

	// Queue for async write (non-blocking if buffer has space)
	select {
	case m.writeChan <- cpCopy:
		return nil
	default:
		// Buffer full, write synchronously
		m.logger.Warn("Checkpoint write buffer full, writing synchronously")
		return m.writeCheckpointToDisk(cpCopy)
	}
}

// SaveSync performs synchronous checkpoint write
func (m *Manager) SaveSync() error {
	if !m.enabled {
		return nil
	}

	m.mu.Lock()
	m.checkpoint.LastSavedAt = time.Now()
	cpCopy := m.copyCheckpoint()
	m.mu.Unlock()

	return m.writeCheckpointToDisk(cpCopy)
}

// copyCheckpoint creates a deep copy of the checkpoint
func (m *Manager) copyCheckpoint() *models.Checkpoint {
	cp := &models.Checkpoint{
		SessionID:         m.checkpoint.SessionID,
		CreatedAt:         m.checkpoint.CreatedAt,
		LastSavedAt:       m.checkpoint.LastSavedAt,
		CurrentPhase:      m.checkpoint.CurrentPhase,
		SubtopicsComplete: m.checkpoint.SubtopicsComplete,
		Subtopics:         append([]string{}, m.checkpoint.Subtopics...),
		PromptsComplete:   m.checkpoint.PromptsComplete,
		Prompts:           append([]models.GenerationJob{}, m.checkpoint.Prompts...),
		CompletedJobIDs:   make(map[int]bool, len(m.checkpoint.CompletedJobIDs)),
		Stats:             m.checkpoint.Stats,
		ConfigHash:        m.checkpoint.ConfigHash,
	}
	for k, v := range m.checkpoint.CompletedJobIDs {
		cp.CompletedJobIDs[k] = v
	}
	return cp
}

// Load reads checkpoint from disk
func Load(sessionDir string, logger *slog.Logger) (*models.Checkpoint, error) {
	checkpointPath := filepath.Join(sessionDir, CheckpointFilename)

	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint: %w", err)
	}

	var cp models.Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	logger.Info("Checkpoint loaded",
		"session_id", cp.SessionID,
		"phase", cp.CurrentPhase,
		"completed_jobs", len(cp.CompletedJobIDs))

	return &cp, nil
}

// MarkSubtopicsComplete saves subtopics phase completion
func (m *Manager) MarkSubtopicsComplete(subtopics []string) error {
	m.mu.Lock()
	m.checkpoint.SubtopicsComplete = true
	m.checkpoint.Subtopics = subtopics
	m.checkpoint.CurrentPhase = models.PhasePrompts
	m.mu.Unlock()

	return m.SaveSync() // Use sync for phase transitions
}

// MarkPromptsComplete saves prompts phase completion
func (m *Manager) MarkPromptsComplete(jobs []models.GenerationJob) error {
	m.mu.Lock()
	m.checkpoint.PromptsComplete = true
	m.checkpoint.Prompts = jobs
	m.checkpoint.CurrentPhase = models.PhasePairs
	m.mu.Unlock()

	return m.SaveSync() // Use sync for phase transitions
}

// MarkJobComplete marks a single job as done (with interval-based saving)
func (m *Manager) MarkJobComplete(jobID int, stats *models.SessionStats) error {
	if !m.enabled {
		return nil
	}

	m.mu.Lock()
	m.checkpoint.CompletedJobIDs[jobID] = true
	m.checkpoint.Stats = *stats
	m.jobCounter++
	shouldSave := m.jobCounter >= m.interval
	if shouldSave {
		m.jobCounter = 0
	}
	m.mu.Unlock()

	if shouldSave {
		return m.Save() // Use async for job completions
	}
	return nil
}

// MarkComplete marks entire generation as complete
func (m *Manager) MarkComplete(stats *models.SessionStats) error {
	m.mu.Lock()
	m.checkpoint.CurrentPhase = models.PhaseComplete
	m.checkpoint.Stats = *stats
	m.mu.Unlock()

	return m.SaveSync() // Use sync for final checkpoint
}

// GetCheckpoint returns a read-only copy of the current checkpoint
func (m *Manager) GetCheckpoint() *models.Checkpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.copyCheckpoint()
}

// Close stops the async writer and waits for pending writes
func (m *Manager) Close() error {
	if !m.enabled {
		return nil
	}

	// Stop the writer goroutine
	close(m.stopWriter)
	m.writeWg.Wait()

	// Check for any write errors
	m.errorMu.Lock()
	defer m.errorMu.Unlock()
	return m.writerError
}

func computeConfigHash(cfg *config.Config) string {
	// Hash critical config fields that affect generation
	data := fmt.Sprintf("%s:%d:%d",
		cfg.Generation.MainTopic,
		cfg.Generation.NumSubtopics,
		cfg.Generation.NumPromptsPerSubtopic)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:8]) // First 8 bytes
}
