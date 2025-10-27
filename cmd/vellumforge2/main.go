package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/lamim/vellumforge2/internal/api"
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

	rootCmd.AddCommand(runCmd)

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

	// Create session manager
	sessionMgr, err := writer.NewSessionManager(slog.Default())
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

	// Create dataset writer
	dataWriter, err := writer.NewDatasetWriter(sessionMgr, logger)
	if err != nil {
		return fmt.Errorf("failed to create dataset writer: %w", err)
	}
	defer func() {
		if err := dataWriter.Close(); err != nil {
			logger.Error("failed to close data writer", "error", err)
		}
	}()

	// Create orchestrator
	orch := orchestrator.New(cfg, secrets, apiClient, dataWriter, logger)

	// Run generation pipeline
	ctx := context.Background()
	if err := orch.Run(ctx); err != nil {
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
