package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/lamim/vellumforge2/internal/api"
	"github.com/lamim/vellumforge2/internal/checkpoint"
	"github.com/lamim/vellumforge2/internal/config"
	"github.com/lamim/vellumforge2/internal/hfhub"
	"github.com/lamim/vellumforge2/internal/orchestrator"
	"github.com/lamim/vellumforge2/internal/writer"
	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

var (
	configPath string
	envFile    string
	uploadToHF bool
	hfRepoID   string
	verbose    bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "vellumforge2",
		Short: "VellumForge2 - Synthetic DPO Dataset Generator",
		Long: `VellumForge2 is a command-line tool for synthetically generating
high-quality Direct Preference Optimization (DPO) datasets using LLMs.`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", Version, GitCommit, BuildTime),
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the dataset generation pipeline",
		Long: `Run the complete dataset generation pipeline:
1. Generate subtopics from main topic
2. Generate prompts for each subtopic
3. Generate preference pairs (chosen/rejected responses)
4. Optional: Evaluate with LLM-as-a-Judge
5. Optional: Upload to Hugging Face Hub`,
		RunE: runGeneration,
	}

	runCmd.Flags().StringVar(&configPath, "config", "config.toml", "Path to configuration file")
	runCmd.Flags().StringVar(&envFile, "env-file", ".env", "Path to environment file")
	runCmd.Flags().BoolVar(&uploadToHF, "upload-to-hf", false, "Upload results to Hugging Face Hub")
	runCmd.Flags().StringVar(&hfRepoID, "hf-repo-id", "", "Hugging Face repository ID (e.g., username/dataset-name)")
	runCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")

	// Checkpoint management commands
	checkpointCmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Manage checkpoints",
		Long:  "Manage generation checkpoints for resuming interrupted sessions",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all available checkpoint sessions",
		Long:  "List all session directories in the output folder that contain checkpoints",
		RunE:  listCheckpoints,
	}

	inspectCmd := &cobra.Command{
		Use:   "inspect <session-dir>",
		Short: "Inspect a checkpoint",
		Long:  "Display detailed information about a checkpoint from a specific session",
		Args:  cobra.ExactArgs(1),
		RunE:  inspectCheckpoint,
	}

	resumeCmd := &cobra.Command{
		Use:   "resume <session-dir>",
		Short: "Resume from a checkpoint",
		Long:  "Resume generation from a specific checkpoint session (automatically updates config)",
		Args:  cobra.ExactArgs(1),
		RunE:  resumeFromCheckpoint,
	}

	resumeCmd.Flags().StringVar(&configPath, "config", "config.toml", "Path to configuration file")
	resumeCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")

	checkpointCmd.AddCommand(listCmd)
	checkpointCmd.AddCommand(inspectCmd)
	checkpointCmd.AddCommand(resumeCmd)

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(checkpointCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runGeneration(cmd *cobra.Command, args []string) error {
	// Load environment variables from file if it exists
	if envFile != "" {
		if err := loadEnvFile(envFile); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load env file: %v\n", err)
		} else if verbose {
			fmt.Fprintf(os.Stderr, "Loaded env file: %s\n", envFile)
		}
	}

	// Load configuration
	cfg, secrets, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Debug: Check if API keys were loaded
	if verbose {
		for provider, key := range secrets.APIKeys {
			if key != "" {
				fmt.Fprintf(os.Stderr, "Loaded API key for: %s (length: %d)\n", provider, len(key))
			}
		}
	}

	// Determine log level
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}

	// Check for resume mode
	resumeMode := cfg.Generation.ResumeFromSession != ""

	// Create session manager (handles both new and resume)
	sessionMgr, err := writer.NewSessionManager(slog.Default(), cfg.Generation.ResumeFromSession)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Set up logger
	logger, logFile, err := writer.SetupLogger(sessionMgr, logLevel)
	if err != nil {
		return fmt.Errorf("failed to setup logger: %w", err)
	}
	defer func() {
		if logFile != nil {
			_ = logFile.Sync()
			_ = logFile.Close()
		}
	}()

	logger.Info("VellumForge2 starting",
		"version", Version,
		"config", configPath,
		"session_dir", sessionMgr.GetSessionDir())

	// Backup config
	if err := sessionMgr.BackupConfig(configPath); err != nil {
		return fmt.Errorf("failed to backup config: %w", err)
	}

	// Create API client
	apiClient := api.NewClient(logger)

	// Set provider-level rate limits if configured
	if len(cfg.ProviderRateLimits) > 0 {
		apiClient.SetProviderRateLimits(cfg.ProviderRateLimits, cfg.ProviderBurstPercent)
		logger.Info("Provider rate limits configured", "providers", cfg.ProviderRateLimits, "burst_percent", cfg.ProviderBurstPercent)
	}

	// Set up checkpoint manager
	var checkpointMgr *checkpoint.Manager

	if resumeMode {
		// Load existing checkpoint
		existingCheckpoint, err := checkpoint.Load(sessionMgr.GetSessionDir(), logger)
		if err != nil {
			return fmt.Errorf("failed to load checkpoint: %w", err)
		}

		// Validate checkpoint
		if err := checkpoint.ValidateCheckpoint(existingCheckpoint, cfg); err != nil {
			return fmt.Errorf("checkpoint validation failed: %w", err)
		}

		checkpointMgr = checkpoint.NewManagerFromCheckpoint(sessionMgr.GetSessionDir(), existingCheckpoint, cfg, logger)
		logger.Info("Loaded checkpoint",
			"phase", existingCheckpoint.CurrentPhase,
			"completed_jobs", len(existingCheckpoint.CompletedJobIDs),
			"progress", fmt.Sprintf("%.1f%%", checkpoint.GetProgressPercentage(existingCheckpoint)))
	} else {
		checkpointMgr = checkpoint.NewManager(sessionMgr.GetSessionDir(), cfg, logger)
	}

	// Create dataset writer (append mode for resume)
	expectedRecords := cfg.Generation.NumSubtopics * cfg.Generation.NumPromptsPerSubtopic
	dataWriter, err := writer.NewDatasetWriter(sessionMgr, logger, resumeMode, expectedRecords)
	if err != nil {
		return fmt.Errorf("failed to create dataset writer: %w", err)
	}
	defer func() {
		if err := dataWriter.Close(); err != nil {
			logger.Error("failed to close data writer", "error", err)
		}
	}()

	// Create orchestrator with checkpoint manager
	orch := orchestrator.New(cfg, secrets, apiClient, dataWriter, checkpointMgr, resumeMode, logger)

	// Run generation pipeline with signal-aware context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := orch.Run(ctx); err != nil {
		if err == context.Canceled {
			sessionDir := filepath.Base(sessionMgr.GetSessionDir())
			logger.Warn("Generation interrupted - resume from checkpoint",
				"session_dir", sessionDir,
				"resume_command", fmt.Sprintf("Set resume_from_session = \"%s\" in config.toml", sessionDir))
			return fmt.Errorf("generation interrupted (resume by setting resume_from_session in config)")
		}
		return fmt.Errorf("generation failed: %w", err)
	}

	// Print stats
	stats := orch.GetStats()
	logger.Info("Generation complete",
		"total_prompts", stats.TotalPrompts,
		"successful", stats.SuccessCount,
		"failed", stats.FailureCount,
		"duration", stats.TotalDuration,
		"session_dir", sessionMgr.GetSessionDir())

	// Optional: Upload to Hugging Face
	if uploadToHF {
		if hfRepoID == "" && cfg.HuggingFace.RepoID == "" {
			return fmt.Errorf("--hf-repo-id must be specified when using --upload-to-hf")
		}

		repoID := hfRepoID
		if repoID == "" {
			repoID = cfg.HuggingFace.RepoID
		}

		if secrets.HuggingFaceToken == "" {
			return fmt.Errorf("HUGGING_FACE_TOKEN environment variable must be set for uploads")
		}

		uploader := hfhub.NewUploader(secrets.HuggingFaceToken, logger)
		if err := uploader.Upload(repoID, sessionMgr.GetSessionDir()); err != nil {
			return fmt.Errorf("upload failed: %w", err)
		}
	}

	logger.Info("All done! 🎉")
	return nil
}

// loadEnvFile loads environment variables from a file
func loadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := splitLines(string(data))
	for _, line := range lines {
		// Skip comments and empty lines
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		// Parse KEY=VALUE
		parts := splitOnce(line, '=')
		if len(parts) == 2 {
			key := trimSpace(parts[0])
			value := trimSpace(parts[1])
			// Remove quotes if present
			value = trimQuotes(value)
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}

	return nil
}

func splitLines(s string) []string {
	var lines []string
	var current []rune
	for _, c := range s {
		if c == '\n' || c == '\r' {
			if len(current) > 0 {
				lines = append(lines, string(current))
				current = nil
			}
		} else {
			current = append(current, c)
		}
	}
	if len(current) > 0 {
		lines = append(lines, string(current))
	}
	return lines
}

func splitOnce(s string, sep rune) []string {
	idx := -1
	for i, c := range s {
		if c == sep {
			idx = i
			break
		}
	}
	if idx == -1 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}

	return s[start:end]
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// listCheckpoints lists all available checkpoint sessions
func listCheckpoints(cmd *cobra.Command, args []string) error {
	outputDir := "output"

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No output directory found. Run a generation first.")
			return nil
		}
		return fmt.Errorf("failed to read output directory: %w", err)
	}

	var sessions []struct {
		name       string
		hasCheckpt bool
		phase      string
		progress   float64
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "session_") {
			continue
		}

		sessionPath := filepath.Join(outputDir, entry.Name())
		checkpointPath := filepath.Join(sessionPath, checkpoint.CheckpointFilename)

		hasCheckpoint := false
		phase := "N/A"
		progress := 0.0

		if _, err := os.Stat(checkpointPath); err == nil {
			hasCheckpoint = true
			// Try to read checkpoint for details
			if cp, err := checkpoint.Load(sessionPath, slog.Default()); err == nil {
				phase = string(cp.CurrentPhase)
				progress = checkpoint.GetProgressPercentage(cp)
			}
		}

		sessions = append(sessions, struct {
			name       string
			hasCheckpt bool
			phase      string
			progress   float64
		}{
			name:       entry.Name(),
			hasCheckpt: hasCheckpoint,
			phase:      phase,
			progress:   progress,
		})
	}

	if len(sessions) == 0 {
		fmt.Println("No session directories found.")
		return nil
	}

	fmt.Println("Available sessions:")
	fmt.Println()
	fmt.Printf("%-35s %-12s %-12s %s\n", "SESSION", "CHECKPOINT", "PHASE", "PROGRESS")
	fmt.Println(strings.Repeat("-", 80))

	for _, s := range sessions {
		checkpointStatus := "No"
		if s.hasCheckpt {
			checkpointStatus = "Yes"
		}
		fmt.Printf("%-35s %-12s %-12s %.1f%%\n", s.name, checkpointStatus, s.phase, s.progress)
	}

	return nil
}

// inspectCheckpoint displays detailed information about a checkpoint
func inspectCheckpoint(cmd *cobra.Command, args []string) error {
	sessionDir := args[0]

	// SECURITY: Validate session path to prevent path traversal (CWE-22)
	if err := writer.ValidateSessionPath(sessionDir); err != nil {
		return fmt.Errorf("invalid session directory: %w", err)
	}

	fullPath := filepath.Join("output", sessionDir)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("session directory not found: %s", sessionDir)
	}

	cp, err := checkpoint.Load(fullPath, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to load checkpoint: %w", err)
	}

	fmt.Printf("Checkpoint Information for: %s\n", sessionDir)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Session ID:          %s\n", cp.SessionID)
	fmt.Printf("Created At:          %s\n", cp.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Last Saved At:       %s\n", cp.LastSavedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Current Phase:       %s\n", cp.CurrentPhase)
	fmt.Printf("Config Hash:         %s\n", cp.ConfigHash)
	fmt.Println()

	fmt.Println("Phase Progress:")
	fmt.Printf("  Subtopics:         %s (%d items)\n", statusStr(cp.SubtopicsComplete), len(cp.Subtopics))
	fmt.Printf("  Prompts:           %s (%d items)\n", statusStr(cp.PromptsComplete), len(cp.Prompts))
	fmt.Printf("  Preference Pairs:  %d / %d completed (%.1f%%)\n",
		checkpoint.GetCompletedCount(cp),
		checkpoint.GetTotalCount(cp),
		checkpoint.GetProgressPercentage(cp))
	fmt.Println()

	fmt.Println("Statistics:")
	fmt.Printf("  Total Prompts:     %d\n", cp.Stats.TotalPrompts)
	fmt.Printf("  Successful:        %d\n", cp.Stats.SuccessCount)
	fmt.Printf("  Failed:            %d\n", cp.Stats.FailureCount)
	fmt.Printf("  Total Duration:    %s\n", cp.Stats.TotalDuration)
	if cp.Stats.SuccessCount > 0 {
		fmt.Printf("  Average Duration:  %s\n", cp.Stats.AverageDuration)
	}
	fmt.Println()

	if cp.CurrentPhase != "complete" {
		fmt.Println("To resume this session, run:")
		fmt.Printf("  Set resume_from_session = \"%s\" in config.toml\n", sessionDir)
		fmt.Printf("  OR use: vellumforge2 checkpoint resume %s\n", sessionDir)
	} else {
		fmt.Println("This session is complete.")
	}

	return nil
}

// resumeFromCheckpoint resumes generation from a checkpoint
func resumeFromCheckpoint(cmd *cobra.Command, args []string) error {
	sessionDir := args[0]

	// SECURITY: Validate session path to prevent path traversal (CWE-22)
	if err := writer.ValidateSessionPath(sessionDir); err != nil {
		return fmt.Errorf("invalid session directory: %w", err)
	}

	fullPath := filepath.Join("output", sessionDir)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("session directory not found: %s", sessionDir)
	}

	// Load and validate checkpoint
	cp, err := checkpoint.Load(fullPath, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to load checkpoint: %w", err)
	}

	if cp.CurrentPhase == "complete" {
		return fmt.Errorf("checkpoint is already complete, nothing to resume")
	}

	// Load config
	cfg, secrets, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate checkpoint compatibility
	if err := checkpoint.ValidateCheckpoint(cp, cfg); err != nil {
		return fmt.Errorf("checkpoint validation failed: %w", err)
	}

	// Set resume mode
	cfg.Generation.ResumeFromSession = sessionDir

	fmt.Printf("Resuming generation from checkpoint: %s\n", sessionDir)
	fmt.Printf("Phase: %s, Progress: %.1f%%\n", cp.CurrentPhase, checkpoint.GetProgressPercentage(cp))
	fmt.Println()

	// Run generation with resume
	return runGenerationWithConfig(cfg, secrets)
}

// runGenerationWithConfig runs generation with provided config
func runGenerationWithConfig(cfg *config.Config, secrets *config.Secrets) error {
	// Determine log level
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}

	// Check for resume mode
	resumeMode := cfg.Generation.ResumeFromSession != ""

	// Create session manager
	sessionMgr, err := writer.NewSessionManager(slog.Default(), cfg.Generation.ResumeFromSession)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Set up logger
	logger, logFile, err := writer.SetupLogger(sessionMgr, logLevel)
	if err != nil {
		return fmt.Errorf("failed to setup logger: %w", err)
	}
	defer func() {
		if logFile != nil {
			_ = logFile.Sync()
			_ = logFile.Close()
		}
	}()

	logger.Info("VellumForge2 starting",
		"version", Version,
		"session_dir", sessionMgr.GetSessionDir(),
		"resume_mode", resumeMode)

	// Backup config if not resuming
	if !resumeMode {
		if err := sessionMgr.BackupConfig(configPath); err != nil {
			return fmt.Errorf("failed to backup config: %w", err)
		}
	}

	// Create API client
	apiClient := api.NewClient(logger)

	// Set provider-level rate limits if configured
	if len(cfg.ProviderRateLimits) > 0 {
		apiClient.SetProviderRateLimits(cfg.ProviderRateLimits, cfg.ProviderBurstPercent)
		logger.Info("Provider rate limits configured", "providers", cfg.ProviderRateLimits, "burst_percent", cfg.ProviderBurstPercent)
	}

	// Set up checkpoint manager
	var checkpointMgr *checkpoint.Manager

	if resumeMode {
		// Load existing checkpoint
		existingCheckpoint, err := checkpoint.Load(sessionMgr.GetSessionDir(), logger)
		if err != nil {
			return fmt.Errorf("failed to load checkpoint: %w", err)
		}

		// Validate checkpoint
		if err := checkpoint.ValidateCheckpoint(existingCheckpoint, cfg); err != nil {
			return fmt.Errorf("checkpoint validation failed: %w", err)
		}

		checkpointMgr = checkpoint.NewManagerFromCheckpoint(sessionMgr.GetSessionDir(), existingCheckpoint, cfg, logger)
		logger.Info("Loaded checkpoint",
			"phase", existingCheckpoint.CurrentPhase,
			"completed_jobs", len(existingCheckpoint.CompletedJobIDs),
			"progress", fmt.Sprintf("%.1f%%", checkpoint.GetProgressPercentage(existingCheckpoint)))
	} else {
		checkpointMgr = checkpoint.NewManager(sessionMgr.GetSessionDir(), cfg, logger)
	}

	// Create dataset writer
	expectedRecords := cfg.Generation.NumSubtopics * cfg.Generation.NumPromptsPerSubtopic
	dataWriter, err := writer.NewDatasetWriter(sessionMgr, logger, resumeMode, expectedRecords)
	if err != nil {
		return fmt.Errorf("failed to create dataset writer: %w", err)
	}
	defer func() {
		if err := dataWriter.Close(); err != nil {
			logger.Error("failed to close data writer", "error", err)
		}
	}()

	// Create orchestrator
	orch := orchestrator.New(cfg, secrets, apiClient, dataWriter, checkpointMgr, resumeMode, logger)

	// Run with context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := orch.Run(ctx); err != nil {
		if err == context.Canceled {
			sessionDirName := filepath.Base(sessionMgr.GetSessionDir())
			logger.Warn("Generation interrupted - resume from checkpoint",
				"session_dir", sessionDirName)
			return fmt.Errorf("generation interrupted (resume by setting resume_from_session in config)")
		}
		return fmt.Errorf("generation failed: %w", err)
	}

	// Print stats
	stats := orch.GetStats()
	logger.Info("Generation complete",
		"total_prompts", stats.TotalPrompts,
		"successful", stats.SuccessCount,
		"failed", stats.FailureCount,
		"duration", stats.TotalDuration,
		"session_dir", sessionMgr.GetSessionDir())

	logger.Info("All done! 🎉")
	return nil
}

func statusStr(complete bool) string {
	if complete {
		return "Complete"
	}
	return "Pending"
}
