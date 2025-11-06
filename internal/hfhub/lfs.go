package hfhub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// LFSPointer represents a pointer to an LFS file
type LFSPointer struct {
	OID    string `json:"oid"`    // SHA256 hash
	Size   int64  `json:"size"`   // File size in bytes
	Path   string `json:"path"`   // Path in repository
	Sample string `json:"sample"` // Sample content for preview (base64)
}

// LFSUploadInfo contains upload information for an LFS file
type LFSUploadInfo struct {
	OID       string            `json:"oid"`
	Size      int64             `json:"size"`
	UploadURL string            `json:"uploadUrl,omitempty"`
	Header    map[string]string `json:"header,omitempty"`
}

// PreuploadRequest represents a request to get LFS upload URLs
type PreuploadRequest struct {
	Files []LFSPointer `json:"files"`
}

// PreuploadResponse represents the response from preupload
type PreuploadResponse struct {
	Files []LFSUploadInfo `json:"files"`
}

// PreuploadLFS requests presigned URLs for uploading LFS files
func (u *Uploader) PreuploadLFS(repoID, branch string, files []LFSPointer) (map[string]*LFSUploadInfo, error) {
	if len(files) == 0 {
		return map[string]*LFSUploadInfo{}, nil
	}

	url := fmt.Sprintf("https://huggingface.co/api/datasets/%s/preupload/%s", repoID, branch)

	payload := PreuploadRequest{Files: files}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+u.token)
	req.Header.Set("Content-Type", "application/json")

	u.logger.Debug("Preupload LFS request", "url", url, "file_count", len(files))

	resp, err := u.preuploadClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			u.logger.Warn("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("preupload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var preuploadResp PreuploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&preuploadResp); err != nil {
		return nil, fmt.Errorf("failed to decode preupload response: %w", err)
	}

	// Create map for easy lookup
	uploadMap := make(map[string]*LFSUploadInfo)
	for i := range preuploadResp.Files {
		uploadMap[preuploadResp.Files[i].OID] = &preuploadResp.Files[i]
	}

	u.logger.Info("Preupload completed", "upload_count", len(uploadMap))
	return uploadMap, nil
}

// UploadLFSFile uploads a file to the presigned S3 URL
func (u *Uploader) UploadLFSFile(uploadInfo *LFSUploadInfo, filePath string) error {
	if uploadInfo.UploadURL == "" {
		// File already exists on server, no upload needed
		// Note: The Upload() function now deletes existing repos to avoid stale LFS cache
		u.logger.Debug("LFS file already exists on server", "oid", uploadInfo.OID)
		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			u.logger.Warn("Failed to close file", "file", filePath, "error", err)
		}
	}()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", uploadInfo.UploadURL, file)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = fileInfo.Size()

	// Add any additional headers from upload info
	for key, value := range uploadInfo.Header {
		req.Header.Set(key, value)
	}

	u.logger.Debug("Uploading LFS file", "oid", uploadInfo.OID, "size", fileInfo.Size())

	resp, err := u.lfsClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			u.logger.Warn("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("LFS upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	u.logger.Info("LFS file uploaded", "oid", uploadInfo.OID, "size", fileInfo.Size())
	return nil
}

// PreuploadLFSWithRetry requests presigned URLs with retry logic
func (u *Uploader) PreuploadLFSWithRetry(repoID, branch string, files []LFSPointer, maxRetries int) (map[string]*LFSUploadInfo, error) {
	var lastErr error
	backoff := 2 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			u.logger.Warn("Retrying LFS preupload",
				"attempt", attempt,
				"max_retries", maxRetries,
				"backoff", backoff)
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}

		result, err := u.PreuploadLFS(repoID, branch, files)
		if err == nil {
			if attempt > 0 {
				u.logger.Info("LFS preupload succeeded after retry", "attempt", attempt)
			}
			return result, nil
		}

		lastErr = err
		u.logger.Warn("LFS preupload failed",
			"attempt", attempt,
			"error", err)
	}

	return nil, fmt.Errorf("preupload failed after %d attempts: %w", maxRetries+1, lastErr)
}

// UploadLFSFileWithRetry uploads a file with retry logic
func (u *Uploader) UploadLFSFileWithRetry(uploadInfo *LFSUploadInfo, filePath string, maxRetries int) error {
	var lastErr error
	backoff := 2 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			u.logger.Warn("Retrying LFS file upload",
				"file", filePath,
				"oid", uploadInfo.OID,
				"attempt", attempt,
				"max_retries", maxRetries,
				"backoff", backoff)
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}

		err := u.UploadLFSFile(uploadInfo, filePath)
		if err == nil {
			if attempt > 0 {
				u.logger.Info("LFS file upload succeeded after retry",
					"file", filePath,
					"attempt", attempt)
			}
			return nil
		}

		lastErr = err
		u.logger.Warn("LFS file upload failed",
			"file", filePath,
			"attempt", attempt,
			"error", err)
	}

	return fmt.Errorf("upload failed after %d attempts: %w", maxRetries+1, lastErr)
}
