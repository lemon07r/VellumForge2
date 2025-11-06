package hfhub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultTimeout is the default timeout for general API operations
	DefaultTimeout = 300 * time.Second
	// PreuploadTimeout is the timeout for LFS preupload requests
	PreuploadTimeout = 300 * time.Second
	// LFSUploadTimeout is the timeout for actual LFS file uploads
	LFSUploadTimeout = 600 * time.Second
	// CommitTimeout is the timeout for commit operations
	CommitTimeout = 300 * time.Second
	// LogPreviewLength is the maximum length for log previews
	LogPreviewLength = 500
	// MaxRetries is the maximum number of retries for failed operations
	MaxRetries = 3
)

// Uploader handles uploading datasets to Hugging Face Hub
type Uploader struct {
	token           string
	httpClient      *http.Client // For general operations
	preuploadClient *http.Client // For LFS preupload
	lfsClient       *http.Client // For LFS file uploads
	commitClient    *http.Client // For commit operations
	logger          *slog.Logger
}

// NewUploader creates a new Hugging Face Hub uploader
func NewUploader(token string, logger *slog.Logger) *Uploader {
	return &Uploader{
		token: token,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		preuploadClient: &http.Client{
			Timeout: PreuploadTimeout,
		},
		lfsClient: &http.Client{
			Timeout: LFSUploadTimeout,
		},
		commitClient: &http.Client{
			Timeout: CommitTimeout,
		},
		logger: logger.With("component", "hf_uploader"),
	}
}

// RepoInfo contains information about a repository
type RepoInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Private bool   `json:"private"`
}

// Upload uploads a session directory to Hugging Face Hub using the commit API
func (u *Uploader) Upload(repoID, sessionDir string) error {
	u.logger.Info("Starting upload to Hugging Face Hub", "repo_id", repoID)

	// Create repository if it doesn't exist
	if err := u.createRepo(repoID); err != nil {
		return fmt.Errorf("failed to create repository: %w", err)
	}

	// Prepare files for upload (dataset + config as vf2.toml)
	filesToUpload := map[string]string{
		"dataset.jsonl":   "dataset.jsonl",
		"config.toml.bak": "vf2.toml", // Rename for clarity on HF Hub
	}
	operations := []CommitOperation{}
	lfsFiles := []LFSPointer{}
	filePaths := make(map[string]string) // oid -> filePath

	// Add .gitattributes to ensure proper text rendering
	// This prevents HuggingFace from adding dataset.jsonl to LFS with -text flag
	gitattributesOp, err := u.createGitAttributesOperation()
	if err != nil {
		u.logger.Warn("Failed to create .gitattributes, continuing without it", "error", err)
	} else {
		operations = append(operations, *gitattributesOp)
		u.logger.Debug("Added .gitattributes to operations")
	}

	for localFilename, hfFilename := range filesToUpload {
		localPath := filepath.Join(sessionDir, localFilename)

		// Check if file exists
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			u.logger.Warn("File not found, skipping", "file", localFilename)
			continue
		}

		// Prepare commit operation with HF filename
		op, err := PrepareFileOperation(localPath, hfFilename)
		if err != nil {
			return fmt.Errorf("failed to prepare %s: %w", localFilename, err)
		}

		operations = append(operations, *op)

		// Track LFS files for upload
		if op.LFSFile != nil {
			// Generate sample preview (first 200 bytes as base64)
			sample, err := generateFileSample(localPath, 200)
			if err != nil {
				u.logger.Warn("Failed to generate sample", "file", localFilename, "error", err)
				sample = "" // Continue without sample
			}

			lfsFiles = append(lfsFiles, LFSPointer{
				OID:    op.LFSFile.SHA256,
				Size:   op.LFSFile.Size,
				Path:   hfFilename,
				Sample: sample,
			})
			filePaths[op.LFSFile.SHA256] = localPath
			u.logger.Debug("File will use LFS", "file", hfFilename, "size", op.LFSFile.Size)
		} else {
			u.logger.Debug("File will be embedded", "file", hfFilename)
		}
	}

	if len(operations) == 0 {
		return fmt.Errorf("no files to upload")
	}

	// Upload LFS files if any
	if len(lfsFiles) > 0 {
		u.logger.Info("Uploading LFS files", "count", len(lfsFiles))

		uploadMap, err := u.PreuploadLFSWithRetry(repoID, "main", lfsFiles, MaxRetries)
		if err != nil {
			return fmt.Errorf("failed to preupload LFS: %w", err)
		}

		for oid, uploadInfo := range uploadMap {
			localPath := filePaths[oid]

			// Empty uploadURL means file already exists on server - skip upload (this is normal!)
			if uploadInfo.UploadURL == "" {
				u.logger.Debug("LFS file already exists on server, skipping upload",
					"oid", oid,
					"file", filepath.Base(localPath))
				continue // Skip upload, file exists
			}

			// Upload the file to S3/storage
			if err := u.UploadLFSFileWithRetry(uploadInfo, localPath, MaxRetries); err != nil {
				return fmt.Errorf("failed to upload LFS file %s: %w", localPath, err)
			}
		}
	}

	// Create commit with retry logic
	sessionName := filepath.Base(sessionDir)
	commitMsg := fmt.Sprintf("Upload dataset from VellumForge2 session %s", sessionName)

	if err := u.createCommitWithRetry(repoID, "main", operations, commitMsg, MaxRetries); err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	u.logger.Info("Upload completed successfully",
		"repo_id", repoID,
		"url", fmt.Sprintf("https://huggingface.co/datasets/%s", repoID))

	return nil
}

func (u *Uploader) deleteRepo(repoID string) error {
	url := fmt.Sprintf("https://huggingface.co/api/datasets/%s", repoID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+u.token)

	u.logger.Debug("Deleting repository", "repo_id", repoID, "url", url)

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			u.logger.Warn("Failed to close response body", "error", err)
		}
	}()

	// 200/204 = success, 404 = already deleted (OK)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete repo failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	u.logger.Info("Repository deleted", "repo_id", repoID)
	return nil
}

func (u *Uploader) createRepo(repoID string) error {
	// Check if repo exists first
	checkURL := fmt.Sprintf("https://huggingface.co/api/datasets/%s", repoID)
	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+u.token)

	resp, err := u.httpClient.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		_ = resp.Body.Close()
		u.logger.Warn("Repository already exists - deleting to ensure clean state", "repo_id", repoID)

		// Delete existing repo to avoid LFS cache issues
		if err := u.deleteRepo(repoID); err != nil {
			return fmt.Errorf("failed to delete existing repo: %w", err)
		}

		// Wait for HF to propagate deletion and clear LFS cache
		// This is necessary because HF's LFS storage is global and cached
		u.logger.Info("Waiting for HF to propagate deletion", "seconds", 10)
		time.Sleep(10 * time.Second)
	} else if resp != nil {
		_ = resp.Body.Close()
	}

	// Create repository
	// Parse username and repo name from repoID (format: "username/reponame")
	parts := strings.Split(repoID, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo_id format, expected 'username/reponame', got '%s'", repoID)
	}
	repoName := parts[1]

	createURL := "https://huggingface.co/api/repos/create"
	payload := map[string]interface{}{
		"name":    repoName,
		"type":    "dataset",
		"private": false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err = http.NewRequest("POST", createURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+u.token)
	req.Header.Set("Content-Type", "application/json")

	u.logger.Debug("Creating repository", "url", createURL, "name", repoName)

	resp, err = u.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			u.logger.Warn("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create repo failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	u.logger.Info("Repository created", "repo_id", repoID)
	return nil
}

func (u *Uploader) createCommit(repoID, branch string, operations []CommitOperation, message string) error {
	url := fmt.Sprintf("https://huggingface.co/api/datasets/%s/commit/%s", repoID, branch)

	// Build NDJSON payload (newline-delimited JSON)
	// Format:
	// {"key": "header", "value": {"summary": "...", "description": ""}}
	// {"key": "file", "value": {"content": "...", "path": "...", "encoding": "base64"}}

	var ndjsonLines []string

	// Header line
	header := map[string]interface{}{
		"key": "header",
		"value": map[string]string{
			"summary":     message,
			"description": "",
		},
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}
	ndjsonLines = append(ndjsonLines, string(headerJSON))

	// File lines
	for _, op := range operations {
		if op.LFSFile != nil {
			// LFS file
			fileLine := map[string]interface{}{
				"key": "lfsFile",
				"value": map[string]interface{}{
					"path": op.Path,
					"algo": "sha256",
					"oid":  op.LFSFile.SHA256,
					"size": op.LFSFile.Size,
				},
			}
			fileJSON, err := json.Marshal(fileLine)
			if err != nil {
				return fmt.Errorf("failed to marshal lfs file %s: %w", op.Path, err)
			}
			ndjsonLines = append(ndjsonLines, string(fileJSON))
		} else {
			// Regular file (base64 encoded)
			fileLine := map[string]interface{}{
				"key": "file",
				"value": map[string]interface{}{
					"content":  op.Content,
					"path":     op.Path,
					"encoding": "base64",
				},
			}
			fileJSON, err := json.Marshal(fileLine)
			if err != nil {
				return fmt.Errorf("failed to marshal file %s: %w", op.Path, err)
			}
			ndjsonLines = append(ndjsonLines, string(fileJSON))
		}
	}

	// Join with newlines to create NDJSON
	ndjsonPayload := strings.Join(ndjsonLines, "\n")

	// Log preview
	if len(ndjsonPayload) > LogPreviewLength {
		u.logger.Debug("Commit payload (NDJSON)", "preview", ndjsonPayload[:LogPreviewLength]+"...")
	} else {
		u.logger.Debug("Commit payload (NDJSON)", "preview", ndjsonPayload)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(ndjsonPayload))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+u.token)
	req.Header.Set("Content-Type", "application/x-ndjson")

	u.logger.Debug("Creating commit", "url", url, "operations", len(operations))
	u.logger.Debug("Starting commit HTTP request")

	resp, err := u.commitClient.Do(req)
	if err != nil {
		u.logger.Warn("Commit HTTP request failed", "error", err)
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			u.logger.Warn("Failed to close response body", "error", err)
		}
	}()

	u.logger.Debug("Received commit HTTP response", "status", resp.StatusCode)

	// Read and log response
	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		u.logger.Error("Commit failed with non-success status",
			"status", resp.StatusCode,
			"response", string(bodyBytes))
		return fmt.Errorf("commit failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	u.logger.Debug("Commit response", "status", resp.StatusCode, "body", string(bodyBytes))
	u.logger.Info("Commit created successfully", "branch", branch, "operations", len(operations))
	return nil
}

// createCommitWithRetry attempts to create a commit with retry logic
func (u *Uploader) createCommitWithRetry(repoID, branch string, operations []CommitOperation, message string, maxRetries int) error {
	var lastErr error
	backoff := 2 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			u.logger.Warn("Retrying commit creation",
				"repo_id", repoID,
				"branch", branch,
				"attempt", attempt,
				"max_retries", maxRetries,
				"backoff", backoff)
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}

		err := u.createCommit(repoID, branch, operations, message)
		if err == nil {
			if attempt > 0 {
				u.logger.Info("Commit creation succeeded after retry",
					"repo_id", repoID,
					"attempt", attempt)
			}
			return nil
		}

		lastErr = err
		u.logger.Warn("Commit creation failed",
			"repo_id", repoID,
			"attempt", attempt,
			"error", err)
	}

	return fmt.Errorf("commit failed after %d attempts: %w", maxRetries+1, lastErr)
}

// createGitAttributesOperation creates a .gitattributes file operation
// that configures proper text handling for JSONL files to ensure
// the HuggingFace viewer renders newlines correctly
// generateFileSample reads the first n bytes of a file and returns base64 encoded
func generateFileSample(filePath string, sampleSize int64) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	buffer := make([]byte, sampleSize)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buffer[:n]), nil
}

// createGitAttributesOperation creates a .gitattributes file operation
func (u *Uploader) createGitAttributesOperation() (*CommitOperation, error) {
	// Create .gitattributes content that:
	// 1. Includes standard HuggingFace LFS patterns
	// 2. Explicitly EXCLUDES .jsonl files from LFS to keep them as regular text
	//    This ensures the dataset viewer renders newlines properly
	content := `*.7z filter=lfs diff=lfs merge=lfs -text
*.arrow filter=lfs diff=lfs merge=lfs -text
*.bin filter=lfs diff=lfs merge=lfs -text
*.bz2 filter=lfs diff=lfs merge=lfs -text
*.ckpt filter=lfs diff=lfs merge=lfs -text
*.ftz filter=lfs diff=lfs merge=lfs -text
*.gz filter=lfs diff=lfs merge=lfs -text
*.h5 filter=lfs diff=lfs merge=lfs -text
*.joblib filter=lfs diff=lfs merge=lfs -text
*.lfs.* filter=lfs diff=lfs merge=lfs -text
*.lz4 filter=lfs diff=lfs merge=lfs -text
*.mds filter=lfs diff=lfs merge=lfs -text
*.mlmodel filter=lfs diff=lfs merge=lfs -text
*.model filter=lfs diff=lfs merge=lfs -text
*.msgpack filter=lfs diff=lfs merge=lfs -text
*.npy filter=lfs diff=lfs merge=lfs -text
*.npz filter=lfs diff=lfs merge=lfs -text
*.onnx filter=lfs diff=lfs merge=lfs -text
*.ot filter=lfs diff=lfs merge=lfs -text
*.parquet filter=lfs diff=lfs merge=lfs -text
*.pb filter=lfs diff=lfs merge=lfs -text
*.pickle filter=lfs diff=lfs merge=lfs -text
*.pkl filter=lfs diff=lfs merge=lfs -text
*.pt filter=lfs diff=lfs merge=lfs -text
*.pth filter=lfs diff=lfs merge=lfs -text
*.rar filter=lfs diff=lfs merge=lfs -text
*.safetensors filter=lfs diff=lfs merge=lfs -text
saved_model/**/* filter=lfs diff=lfs merge=lfs -text
*.tar.* filter=lfs diff=lfs merge=lfs -text
*.tar filter=lfs diff=lfs merge=lfs -text
*.tflite filter=lfs diff=lfs merge=lfs -text
*.tgz filter=lfs diff=lfs merge=lfs -text
*.wasm filter=lfs diff=lfs merge=lfs -text
*.xz filter=lfs diff=lfs merge=lfs -text
*.zip filter=lfs diff=lfs merge=lfs -text
*.zst filter=lfs diff=lfs merge=lfs -text
*tfevents* filter=lfs diff=lfs merge=lfs -text
# Audio files - uncompressed
*.pcm filter=lfs diff=lfs merge=lfs -text
*.sam filter=lfs diff=lfs merge=lfs -text
*.raw filter=lfs diff=lfs merge=lfs -text
# Audio files - compressed
*.aac filter=lfs diff=lfs merge=lfs -text
*.flac filter=lfs diff=lfs merge=lfs -text
*.mp3 filter=lfs diff=lfs merge=lfs -text
*.ogg filter=lfs diff=lfs merge=lfs -text
*.wav filter=lfs diff=lfs merge=lfs -text
# Image files - uncompressed
*.bmp filter=lfs diff=lfs merge=lfs -text
*.gif filter=lfs diff=lfs merge=lfs -text
*.png filter=lfs diff=lfs merge=lfs -text
*.tiff filter=lfs diff=lfs merge=lfs -text
# Image files - compressed
*.jpg filter=lfs diff=lfs merge=lfs -text
*.jpeg filter=lfs diff=lfs merge=lfs -text
*.webp filter=lfs diff=lfs merge=lfs -text
# Video files - compressed
*.mp4 filter=lfs diff=lfs merge=lfs -text
*.webm filter=lfs diff=lfs merge=lfs -text
# IMPORTANT: Exclude JSONL files from LFS to ensure proper text rendering
# JSONL datasets should be treated as regular text files so the HuggingFace
# viewer can properly render newlines and make the data human-readable
# *.jsonl is deliberately NOT included in LFS
`

	// Encode as base64
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	op := &CommitOperation{
		Operation: "add",
		Path:      ".gitattributes",
		Content:   encoded,
		Encoding:  "base64",
	}

	u.logger.Info("Created .gitattributes configuration",
		"excludes_jsonl", true,
		"reason", "Ensures proper newline rendering in HuggingFace viewer")

	return op, nil
}
