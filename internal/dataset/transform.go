package dataset

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"

	"github.com/lamim/vellumforge2/internal/api"
	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/internal/util"
	"github.com/lamim/vellumforge2/pkg/models"
)

// TransformMode specifies what kind of dataset operation to perform.
type TransformMode string

const (
	// TransformSFTToDPO converts an SFT dataset into a DPO dataset by generating rejected responses.
	TransformSFTToDPO TransformMode = "sft-to-dpo"
	// TransformRegenRejected regenerates the rejected responses for an existing DPO dataset.
	TransformRegenRejected TransformMode = "regen-rejected"
)

// Options controls dataset transformation behaviour.
type Options struct {
	InputPath           string
	OutputPath          string
	InputReasoningPath  string
	OutputReasoningPath string

	// Concurrency controls how many rejected generations run in parallel.
	Concurrency int

	// CheckpointPath is where progress is saved (defaults to <output>.checkpoint.json).
	CheckpointPath string
	// Resume indicates that we should resume from an existing checkpoint instead of starting fresh.
	Resume bool
	// CheckpointInterval controls how often (in completed jobs) we persist progress.
	CheckpointInterval int
}

// transformCheckpoint tracks progress for long-running transforms so they can be resumed.
type transformCheckpoint struct {
	Mode                TransformMode `json:"mode"`
	InputPath           string        `json:"input_path"`
	OutputPath          string        `json:"output_path"`
	InputReasoningPath  string        `json:"input_reasoning_path,omitempty"`
	OutputReasoningPath string        `json:"output_reasoning_path,omitempty"`
	TotalJobs           int           `json:"total_jobs"`
	CompletedJobs       int           `json:"completed_jobs"`
	LastUpdated         time.Time     `json:"last_updated"`
}

// Run performs a dataset transformation using the provided config and API client.
//
// It supports two modes:
//   - TransformSFTToDPO: read SFT records and write DPO records with newly generated rejected responses
//   - TransformRegenRejected: read DPO records and regenerate the rejected field using the rejected model
func Run(
	ctx context.Context,
	logger *slog.Logger,
	mode TransformMode,
	cfg *config.Config,
	secrets *config.Secrets,
	client *api.Client,
	opts Options,
) error {
	// Validate required paths based on mode
	switch mode {
	case TransformSFTToDPO:
		if opts.InputPath == "" {
			return fmt.Errorf("input path is required for sft-to-dpo mode")
		}
		if opts.OutputPath == "" {
			return fmt.Errorf("output path is required for sft-to-dpo mode")
		}
	case TransformRegenRejected:
		if opts.InputPath == "" && opts.InputReasoningPath == "" {
			return fmt.Errorf("at least one of input or input_reasoning path is required for regen-rejected mode")
		}
		if opts.OutputPath == "" && opts.OutputReasoningPath == "" {
			return fmt.Errorf("at least one of output or output_reasoning path is required for regen-rejected mode")
		}
		if opts.OutputReasoningPath != "" && opts.InputReasoningPath == "" {
			return fmt.Errorf("output_reasoning requires input_reasoning for regen-rejected mode")
		}
	default:
		return fmt.Errorf("unsupported transform mode: %s", mode)
	}
	// Ensure rejected model is configured; both modes rely on it.
	rejectedModel, ok := cfg.Models["rejected"]
	if !ok {
		return fmt.Errorf("config is missing 'rejected' model; it is required for dataset transforms")
	}

	if cfg.PromptTemplates.RejectedGeneration == "" {
		return fmt.Errorf("config.prompt_templates.rejected_generation is required for dataset transforms")
	}

	// Apply defaults for concurrency and checkpointing.
	if opts.Concurrency <= 0 {
		opts.Concurrency = cfg.Generation.Concurrency
		if opts.Concurrency <= 0 {
			opts.Concurrency = 4
		}
	}
	if opts.CheckpointInterval <= 0 {
		if cfg.Generation.CheckpointInterval > 0 {
			opts.CheckpointInterval = cfg.Generation.CheckpointInterval
		} else {
			opts.CheckpointInterval = 10
		}
	}
	if opts.CheckpointPath == "" {
		base := opts.OutputPath
		if base == "" {
			base = opts.OutputReasoningPath
		}
		opts.CheckpointPath = base + ".checkpoint.json"
	}

	// Ensure output directories exist (mirrors behaviour of main pipeline).
	if opts.OutputPath != "" {
		if dir := filepath.Dir(opts.OutputPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}
		}
	}
	if opts.OutputReasoningPath != "" {
		if dir := filepath.Dir(opts.OutputReasoningPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("failed to create reasoning output directory: %w", err)
			}
		}
	}

	switch mode {
	case TransformSFTToDPO:
		return runSFTToDPO(ctx, logger, cfg, secrets, client, rejectedModel, opts)
	case TransformRegenRejected:
		return runRegenRejected(ctx, logger, cfg, secrets, client, rejectedModel, opts)
	default:
		return fmt.Errorf("unsupported transform mode: %s", mode)
	}
}

// runSFTToDPO converts an SFT dataset into a DPO dataset by generating new rejected responses.
// It uses a worker pool for concurrency and a lightweight checkpoint for resume support.
func runSFTToDPO(
	parentCtx context.Context,
	logger *slog.Logger,
	cfg *config.Config,
	secrets *config.Secrets,
	client *api.Client,
	rejectedModel config.ModelConfig,
	opts Options,
) error {
	jobs, err := buildSFTJobs(opts.InputPath, cfg.Generation.SFTFormat)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		logger.Info("No SFT records found in input dataset", "input", opts.InputPath)
		return nil
	}

	cp, err := initTransformCheckpoint(opts, TransformSFTToDPO, len(jobs))
	if err != nil {
		return err
	}
	if cp.CompletedJobs >= cp.TotalJobs {
		logger.Info("Transform already complete", "input", opts.InputPath, "output", opts.OutputPath)
		return nil
	}

	bar := progressbar.Default(int64(cp.TotalJobs), "Transforming SFT→DPO")
	if cp.CompletedJobs > 0 {
		if cp.CompletedJobs > cp.TotalJobs {
			cp.CompletedJobs = cp.TotalJobs
		}
		_ = bar.Add(cp.CompletedJobs)
	}

	// Open output file (append on resume, create otherwise).
	var outputFile *os.File
	if opts.Resume {
		outputFile, err = os.OpenFile(opts.OutputPath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			if os.IsNotExist(err) {
				outputFile, err = os.Create(opts.OutputPath)
			}
			if err != nil {
				return fmt.Errorf("failed to open output dataset for append: %w", err)
			}
		}
	} else {
		outputFile, err = os.Create(opts.OutputPath)
		if err != nil {
			return fmt.Errorf("failed to create output dataset: %w", err)
		}
	}
	defer func() { _ = outputFile.Close() }()

	encoder := json.NewEncoder(outputFile)
	apiKey := secrets.GetAPIKey(rejectedModel.BaseURL)
	if apiKey == "" && !isLocalEndpoint(rejectedModel.BaseURL) {
		logger.Warn("No API key found for rejected model base URL", "base_url", rejectedModel.BaseURL)
	}

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	jobCh := make(chan sftJob)
	resultCh := make(chan sftResult)
	var wg sync.WaitGroup

	// Start worker pool
	for i := 0; i < opts.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobCh:
					if !ok {
						return
					}

					rejected, err := generateRejected(ctx, logger, cfg, client, rejectedModel, apiKey, job.Prompt)
					res := sftResult{Job: job, Rejected: rejected, Err: err}

					select {
					case <-ctx.Done():
						return
					case resultCh <- res:
					}
				}
			}
		}(i)
	}

	// Feed jobs starting from the next incomplete index.
	go func() {
		defer close(jobCh)
		for i := cp.CompletedJobs; i < len(jobs); i++ {
			select {
			case <-ctx.Done():
				return
			case jobCh <- jobs[i]:
			}
		}
	}()

	// Close resultCh when all workers are done.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	nextID := cp.CompletedJobs
	pending := make(map[int]sftResult)

	for res := range resultCh {
		if res.Err != nil {
			cancel()
			return fmt.Errorf("job %d (line %d) failed: %w", res.Job.ID, res.Job.LineNumber, res.Err)
		}

		pending[res.Job.ID] = res

		for {
			nextRes, ok := pending[nextID]
			if !ok {
				break
			}

			record := models.DPORecord{
				Prompt:   nextRes.Job.Prompt,
				Chosen:   nextRes.Job.Chosen,
				Rejected: nextRes.Rejected,
			}
			if err := encoder.Encode(&record); err != nil {
				cancel()
				return fmt.Errorf("failed to write DPO record for job %d (line %d): %w", nextRes.Job.ID, nextRes.Job.LineNumber, err)
			}

			delete(pending, nextID)
			nextID++
			cp.CompletedJobs++
			cp.LastUpdated = time.Now()
			_ = bar.Add(1)

			if opts.CheckpointInterval > 0 && cp.CompletedJobs%opts.CheckpointInterval == 0 {
				if err := saveTransformCheckpoint(opts.CheckpointPath, cp); err != nil {
					logger.Warn("Failed to save transform checkpoint", "error", err)
				}
			}
		}
	}

	if err := parentCtx.Err(); err != nil && err != context.Canceled {
		return err
	}

	// Final checkpoint save
	if err := saveTransformCheckpoint(opts.CheckpointPath, cp); err != nil {
		logger.Warn("Failed to save final transform checkpoint", "error", err)
	}

	logger.Info("SFT→DPO transform completed",
		"input", opts.InputPath,
		"output", opts.OutputPath,
		"total_jobs", cp.TotalJobs,
		"completed_jobs", cp.CompletedJobs)

	return nil
}

// runRegenRejected regenerates the rejected responses of an existing DPO dataset.
// It uses the same worker-pool + checkpoint pattern as runSFTToDPO.
func runRegenRejected(
	parentCtx context.Context,
	logger *slog.Logger,
	cfg *config.Config,
	secrets *config.Secrets,
	client *api.Client,
	rejectedModel config.ModelConfig,
	opts Options,
) error {
	var (
		jobs          []dpoJob
		reasoningRecs []reasoningDPORecord
		err           error
	)

	// If a reasoning dataset is provided, use it as the primary source of truth
	// for prompts/chosen and for preserving rejected reasoning.
	if opts.InputReasoningPath != "" {
		reasoningRecs, err = loadReasoningDPO(opts.InputReasoningPath)
		if err != nil {
			return err
		}
		if len(reasoningRecs) == 0 {
			logger.Info("No DPO records found in reasoning input dataset", "input_reasoning", opts.InputReasoningPath)
			return nil
		}

		jobs = make([]dpoJob, len(reasoningRecs))
		for i, rr := range reasoningRecs {
			prompt := strings.TrimSpace(rr.Record.Prompt)
			if prompt == "" {
				return fmt.Errorf("line %d: DPO reasoning record is missing prompt field", rr.LineNumber)
			}

			chosen := rr.Record.Chosen
			if util.ContainsThinkTags(chosen) {
				// Drop reasoning from chosen when reconstructing the non-reasoning dataset
				_, answer := util.SplitThinkAndAnswer(chosen)
				if strings.TrimSpace(answer) != "" {
					chosen = answer
				}
			}

			jobs[i] = dpoJob{
				ID:         i,
				LineNumber: rr.LineNumber,
				Prompt:     prompt,
				Chosen:     chosen,
			}
		}
	} else {
		// Fallback: use non-reasoning dataset only
		jobs, err = buildDPOJobs(opts.InputPath)
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			logger.Info("No DPO records found in input dataset", "input", opts.InputPath)
			return nil
		}
	}

	cp, err := initTransformCheckpoint(opts, TransformRegenRejected, len(jobs))
	if err != nil {
		return err
	}
	if cp.CompletedJobs >= cp.TotalJobs {
		logger.Info("Transform already complete", "input", opts.InputPath, "output", opts.OutputPath)
		return nil
	}

	bar := progressbar.Default(int64(cp.TotalJobs), "Regenerating rejected")
	if cp.CompletedJobs > 0 {
		if cp.CompletedJobs > cp.TotalJobs {
			cp.CompletedJobs = cp.TotalJobs
		}
		_ = bar.Add(cp.CompletedJobs)
	}

	// Open output files (append on resume, create otherwise).
	var outputFile *os.File
	var reasoningFile *os.File

	if opts.OutputPath != "" {
		if opts.Resume {
			outputFile, err = os.OpenFile(opts.OutputPath, os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				if os.IsNotExist(err) {
					outputFile, err = os.Create(opts.OutputPath)
				}
				if err != nil {
					return fmt.Errorf("failed to open output dataset for append: %w", err)
				}
			}
		} else {
			outputFile, err = os.Create(opts.OutputPath)
			if err != nil {
				return fmt.Errorf("failed to create output dataset: %w", err)
			}
		}
		defer func() { _ = outputFile.Close() }()
	}

	if opts.OutputReasoningPath != "" {
		if opts.Resume {
			reasoningFile, err = os.OpenFile(opts.OutputReasoningPath, os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				if os.IsNotExist(err) {
					reasoningFile, err = os.Create(opts.OutputReasoningPath)
				}
				if err != nil {
					return fmt.Errorf("failed to open reasoning output dataset for append: %w", err)
				}
			}
		} else {
			reasoningFile, err = os.Create(opts.OutputReasoningPath)
			if err != nil {
				return fmt.Errorf("failed to create reasoning output dataset: %w", err)
			}
		}
		defer func() { _ = reasoningFile.Close() }()
	}

	var encoder *json.Encoder
	if outputFile != nil {
		encoder = json.NewEncoder(outputFile)
	}
	var reasoningEncoder *json.Encoder
	if reasoningFile != nil {
		reasoningEncoder = json.NewEncoder(reasoningFile)
	}
	apiKey := secrets.GetAPIKey(rejectedModel.BaseURL)
	if apiKey == "" && !isLocalEndpoint(rejectedModel.BaseURL) {
		logger.Warn("No API key found for rejected model base URL", "base_url", rejectedModel.BaseURL)
	}

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	jobCh := make(chan dpoJob)
	resultCh := make(chan dpoResult)
	var wg sync.WaitGroup

	// Start worker pool
	for i := 0; i < opts.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobCh:
					if !ok {
						return
					}

					rejected, err := generateRejected(ctx, logger, cfg, client, rejectedModel, apiKey, job.Prompt)
					res := dpoResult{Job: job, Rejected: rejected, Err: err}

					select {
					case <-ctx.Done():
						return
					case resultCh <- res:
					}
				}
			}
		}(i)
	}

	// Feed jobs starting from the next incomplete index.
	go func() {
		defer close(jobCh)
		for i := cp.CompletedJobs; i < len(jobs); i++ {
			select {
			case <-ctx.Done():
				return
			case jobCh <- jobs[i]:
			}
		}
	}()

	// Close resultCh when all workers are done.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	nextID := cp.CompletedJobs
	pending := make(map[int]dpoResult)

	for res := range resultCh {
		if res.Err != nil {
			cancel()
			return fmt.Errorf("job %d (line %d) failed: %w", res.Job.ID, res.Job.LineNumber, res.Err)
		}

		pending[res.Job.ID] = res

		for {
			nextRes, ok := pending[nextID]
			if !ok {
				break
			}

			// Write non-reasoning dataset if requested
			if encoder != nil {
				record := models.DPORecord{
					Prompt:   nextRes.Job.Prompt,
					Chosen:   nextRes.Job.Chosen,
					Rejected: nextRes.Rejected,
				}
				if err := encoder.Encode(&record); err != nil {
					cancel()
					return fmt.Errorf("failed to write updated DPO record for job %d (line %d): %w", nextRes.Job.ID, nextRes.Job.LineNumber, err)
				}
			}

			// Write reasoning dataset if requested
			if reasoningEncoder != nil && len(reasoningRecs) > 0 {
				if nextRes.Job.ID < 0 || nextRes.Job.ID >= len(reasoningRecs) {
					cancel()
					return fmt.Errorf("invalid reasoning record index %d for job %d", nextRes.Job.ID, nextRes.Job.ID)
				}
				base := reasoningRecs[nextRes.Job.ID].Record

				// Preserve existing reasoning (if any) while updating the rejected content
				combinedRejected := nextRes.Rejected
				if base.Rejected != "" && util.ContainsThinkTags(base.Rejected) {
					think, _ := util.SplitThinkAndAnswer(base.Rejected)
					if strings.TrimSpace(think) != "" {
						combinedRejected = util.CombineReasoningAndContent(think, nextRes.Rejected)
					}
				}

				reasoningRecord := models.DPORecord{
					Prompt:   base.Prompt,
					Chosen:   base.Chosen,
					Rejected: combinedRejected,
				}
				if err := reasoningEncoder.Encode(&reasoningRecord); err != nil {
					cancel()
					return fmt.Errorf("failed to write reasoning DPO record for job %d (line %d): %w", nextRes.Job.ID, nextRes.Job.LineNumber, err)
				}
			}

			delete(pending, nextID)
			nextID++
			cp.CompletedJobs++
			cp.LastUpdated = time.Now()
			_ = bar.Add(1)

			if opts.CheckpointInterval > 0 && cp.CompletedJobs%opts.CheckpointInterval == 0 {
				if err := saveTransformCheckpoint(opts.CheckpointPath, cp); err != nil {
					logger.Warn("Failed to save transform checkpoint", "error", err)
				}
			}
		}
	}

	if err := parentCtx.Err(); err != nil && err != context.Canceled {
		return err
	}

	// Final checkpoint save
	if err := saveTransformCheckpoint(opts.CheckpointPath, cp); err != nil {
		logger.Warn("Failed to save final transform checkpoint", "error", err)
	}

	logger.Info("DPO rejected regeneration completed",
		"input", opts.InputPath,
		"output", opts.OutputPath,
		"total_jobs", cp.TotalJobs,
		"completed_jobs", cp.CompletedJobs)

	return nil
}

// extractPromptAndChosen derives the prompt and chosen text from an SFT record
// using the configured SFT format.
func extractPromptAndChosen(record *models.SFTRecord, format models.SFTFormat) (string, string, error) {
	switch format {
	case models.SFTFormatAlpaca:
		prompt := strings.TrimSpace(record.Instruction)
		if record.Input != "" {
			if prompt != "" {
				prompt += "\n\n" + strings.TrimSpace(record.Input)
			} else {
				prompt = strings.TrimSpace(record.Input)
			}
		}
		if prompt == "" {
			return "", "", fmt.Errorf("alpaca SFT record missing instruction/input")
		}
		if strings.TrimSpace(record.Output) == "" {
			return "", "", fmt.Errorf("alpaca SFT record missing output")
		}
		return prompt, record.Output, nil

	case models.SFTFormatShareGPT:
		var prompt string
		var chosen string

		for _, msg := range record.Conversations {
			from := strings.ToLower(strings.TrimSpace(msg.From))
			if prompt == "" && (from == "human" || from == "user") {
				prompt = strings.TrimSpace(msg.Value)
			}
		}

		for i := len(record.Conversations) - 1; i >= 0; i-- {
			from := strings.ToLower(strings.TrimSpace(record.Conversations[i].From))
			if from == "gpt" || from == "assistant" {
				chosen = record.Conversations[i].Value
				break
			}
		}

		if prompt == "" {
			return "", "", fmt.Errorf("sharegpt SFT record missing human prompt")
		}
		if strings.TrimSpace(chosen) == "" {
			return "", "", fmt.Errorf("sharegpt SFT record missing assistant completion")
		}
		return prompt, chosen, nil

	default:
		return "", "", fmt.Errorf("unsupported SFT format for transform: %s", format)
	}
}

// sftJob represents a single SFT example to be converted to a DPO record.
type sftJob struct {
	ID         int
	LineNumber int
	Prompt     string
	Chosen     string
}

type sftResult struct {
	Job      sftJob
	Rejected string
	Err      error
}

// dpoJob represents a single DPO example whose rejected response should be regenerated.
type dpoJob struct {
	ID         int
	LineNumber int
	Prompt     string
	Chosen     string
}

type dpoResult struct {
	Job      dpoJob
	Rejected string
	Err      error
}

// reasoningDPORecord keeps the original reasoning-aware record and its source line number.
type reasoningDPORecord struct {
	Record     models.DPORecord
	LineNumber int
}

// buildSFTJobs parses an input SFT JSONL file into a list of jobs.
func buildSFTJobs(inputPath string, format models.SFTFormat) ([]sftJob, error) {
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open input dataset: %w", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)

	var jobs []sftJob
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var sft models.SFTRecord
		if err := json.Unmarshal([]byte(line), &sft); err != nil {
			return nil, fmt.Errorf("line %d: failed to parse SFT record: %w", lineNum, err)
		}

		prompt, chosen, err := extractPromptAndChosen(&sft, format)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}

		id := len(jobs)
		jobs = append(jobs, sftJob{
			ID:         id,
			LineNumber: lineNum,
			Prompt:     prompt,
			Chosen:     chosen,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed while reading input dataset: %w", err)
	}

	return jobs, nil
}

// buildDPOJobs parses an input DPO JSONL file into a list of jobs.
func buildDPOJobs(inputPath string) ([]dpoJob, error) {
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open input dataset: %w", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)

	var jobs []dpoJob
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var dpo models.DPORecord
		if err := json.Unmarshal([]byte(line), &dpo); err != nil {
			return nil, fmt.Errorf("line %d: failed to parse DPO record: %w", lineNum, err)
		}

		if strings.TrimSpace(dpo.Prompt) == "" {
			return nil, fmt.Errorf("line %d: DPO record is missing prompt field", lineNum)
		}
		if strings.TrimSpace(dpo.Chosen) == "" {
			return nil, fmt.Errorf("line %d: DPO record is missing chosen field", lineNum)
		}

		id := len(jobs)
		jobs = append(jobs, dpoJob{
			ID:         id,
			LineNumber: lineNum,
			Prompt:     dpo.Prompt,
			Chosen:     dpo.Chosen,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed while reading input dataset: %w", err)
	}

	return jobs, nil
}

// loadReasoningDPO loads a reasoning-aware DPO dataset into memory.
func loadReasoningDPO(inputPath string) ([]reasoningDPORecord, error) {
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open reasoning input dataset: %w", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)

	var records []reasoningDPORecord
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var dpo models.DPORecord
		if err := json.Unmarshal([]byte(line), &dpo); err != nil {
			return nil, fmt.Errorf("line %d: failed to parse reasoning DPO record: %w", lineNum, err)
		}

		records = append(records, reasoningDPORecord{
			Record:     dpo,
			LineNumber: lineNum,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed while reading reasoning input dataset: %w", err)
	}

	return records, nil
}

// generateRejected calls the rejected model using the configured rejected_generation template.
func generateRejected(
	ctx context.Context,
	logger *slog.Logger,
	cfg *config.Config,
	client *api.Client,
	rejectedModel config.ModelConfig,
	apiKey string,
	prompt string,
) (string, error) {
	renderedPrompt, err := util.RenderTemplate(cfg.PromptTemplates.RejectedGeneration, map[string]interface{}{
		"Prompt": prompt,
	})
	if err != nil {
		return "", fmt.Errorf("failed to render rejected template: %w", err)
	}

	messages := []api.Message{}
	if cfg.PromptTemplates.RejectedSystemPrompt != "" {
		messages = append(messages, api.Message{
			Role:    "system",
			Content: cfg.PromptTemplates.RejectedSystemPrompt,
		})
	}
	messages = append(messages, api.Message{
		Role:    "user",
		Content: renderedPrompt,
	})

	var resp *api.ChatCompletionResponse
	if rejectedModel.UseStreaming {
		resp, err = client.ChatCompletionStreaming(ctx, rejectedModel, apiKey, messages)
	} else {
		resp, err = client.ChatCompletion(ctx, rejectedModel, apiKey, messages)
	}
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from rejected model")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("empty content returned from rejected model")
	}
	cleaned := util.CleanMetaFromLLMResponse(content)
	if cleaned != "" {
		content = cleaned
	}

	return content, nil
}

// initTransformCheckpoint loads or creates a checkpoint for a transform run.
func initTransformCheckpoint(opts Options, mode TransformMode, totalJobs int) (*transformCheckpoint, error) {
	if opts.Resume {
		cp, err := loadTransformCheckpoint(opts.CheckpointPath)
		if err != nil {
			return nil, err
		}
		if cp.Mode != mode {
			return nil, fmt.Errorf("checkpoint mode mismatch: expected %s, got %s", mode, cp.Mode)
		}
		if cp.InputPath != opts.InputPath || cp.OutputPath != opts.OutputPath ||
			cp.InputReasoningPath != opts.InputReasoningPath || cp.OutputReasoningPath != opts.OutputReasoningPath {
			return nil, fmt.Errorf("checkpoint I/O mismatch: checkpoint was created for input=%s, output=%s, input_reasoning=%s, output_reasoning=%s",
				cp.InputPath, cp.OutputPath, cp.InputReasoningPath, cp.OutputReasoningPath)
		}
		// If totalJobs changed (e.g. edited dataset), prefer the current scan but keep completed count bounded.
		cp.TotalJobs = totalJobs
		if cp.CompletedJobs > cp.TotalJobs {
			cp.CompletedJobs = cp.TotalJobs
		}
		return cp, nil
	}

	cp := &transformCheckpoint{
		Mode:                mode,
		InputPath:           opts.InputPath,
		OutputPath:          opts.OutputPath,
		InputReasoningPath:  opts.InputReasoningPath,
		OutputReasoningPath: opts.OutputReasoningPath,
		TotalJobs:           totalJobs,
		CompletedJobs:       0,
		LastUpdated:         time.Now(),
	}
	if err := saveTransformCheckpoint(opts.CheckpointPath, cp); err != nil {
		return nil, err
	}
	return cp, nil
}

func loadTransformCheckpoint(path string) (*transformCheckpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read transform checkpoint: %w", err)
	}
	var cp transformCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transform checkpoint: %w", err)
	}
	return &cp, nil
}

func saveTransformCheckpoint(path string, cp *transformCheckpoint) error {
	cp.LastUpdated = time.Now()
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal transform checkpoint: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write transform checkpoint: %w", err)
	}
	return nil
}

// isLocalEndpoint mirrors the internal api helper for determining if a base URL is local.
func isLocalEndpoint(endpoint string) bool {
	return strings.Contains(endpoint, "://127.0.0.1") ||
		strings.Contains(endpoint, "://localhost") ||
		strings.Contains(endpoint, "://[::1]") ||
		strings.Contains(endpoint, "://0.0.0.0")
}
