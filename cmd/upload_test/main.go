package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/lamim/vellumforge2/internal/hfhub"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <repo-id> <session-dir>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s lemon07r/VellumK2-Fantasy-DPO-Large-01 output/session_2025-11-06T06-49-09\n", os.Args[0])
		os.Exit(1)
	}

	repoID := os.Args[1]
	sessionDir := os.Args[2]

	// Get token from environment
	token := os.Getenv("HUGGINGFACE_TOKEN")
	if token == "" {
		token = os.Getenv("HF_TOKEN")
	}
	if token == "" {
		fmt.Fprintf(os.Stderr, "Error: HUGGINGFACE_TOKEN environment variable not set\n")
		os.Exit(1)
	}

	// Check if session directory exists
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Session directory not found: %s\n", sessionDir)
		os.Exit(1)
	}

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create uploader
	uploader := hfhub.NewUploader(token, logger)

	fmt.Printf("Uploading session to HuggingFace Hub...\n")
	fmt.Printf("  Repository: %s\n", repoID)
	fmt.Printf("  Session:    %s\n", sessionDir)
	fmt.Println()

	// Upload
	if err := uploader.Upload(repoID, sessionDir); err != nil {
		fmt.Fprintf(os.Stderr, "Upload failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("✓ Upload completed successfully!")
	fmt.Printf("  View at: https://huggingface.co/datasets/%s\n", repoID)
}
