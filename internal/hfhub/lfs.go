package hfhub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"time"
)

// LFSPointer represents a pointer to an LFS file (used for tracking)
type LFSPointer struct {
	OID    string `json:"oid"`    // SHA256 hash
	Size   int64  `json:"size"`   // File size in bytes
	Path   string `json:"path"`   // Path in repository (not sent to LFS API)
	Sample string `json:"sample"` // Sample content for preview (not sent to LFS API)
}

// LFSUploadInfo contains upload information for an LFS file
type LFSUploadInfo struct {
	OID       string            `json:"oid"`
	Size      int64             `json:"size"`
	UploadURL string            `json:"-"` // Populated from actions.upload.href
	Header    map[string]string `json:"-"` // Populated from actions.upload.header
}

// LFSBatchObject represents an object in the LFS batch request/response
type LFSBatchObject struct {
	OID     string      `json:"oid"`               // SHA256 hex string
	Size    int64       `json:"size"`              // File size in bytes
	Actions *LFSActions `json:"actions,omitempty"` // Upload/verify actions (nil if file exists)
}

// LFSActions contains upload and verify actions
type LFSActions struct {
	Upload *LFSAction `json:"upload,omitempty"`
	Verify *LFSAction `json:"verify,omitempty"`
}

// LFSAction represents an upload or verify action
type LFSAction struct {
	Href   string            `json:"href"`   // URL to upload/verify
	Header map[string]string `json:"header"` // HTTP headers to include
}

// LFSBatchRequest is the request to the LFS batch endpoint
type LFSBatchRequest struct {
	Operation string           `json:"operation"` // Always "upload"
	Transfers []string         `json:"transfers"` // ["basic", "multipart"]
	Objects   []LFSBatchObject `json:"objects"`   // Files to upload
	HashAlgo  string           `json:"hash_algo"` // Always "sha256"
}

// LFSBatchResponse is the response from the LFS batch endpoint
type LFSBatchResponse struct {
	Objects  []LFSBatchObject `json:"objects"`            // Upload instructions
	Transfer string           `json:"transfer,omitempty"` // Chosen transfer method
}

// PreuploadLFS requests presigned URLs for uploading LFS files using Git LFS batch API
func (u *Uploader) PreuploadLFS(repoID, branch string, files []LFSPointer) (map[string]*LFSUploadInfo, error) {
	if len(files) == 0 {
		return map[string]*LFSUploadInfo{}, nil
	}

	// Use correct Git LFS batch endpoint: {repo}.git/info/lfs/objects/batch
	url := fmt.Sprintf("https://huggingface.co/datasets/%s.git/info/lfs/objects/batch", repoID)

	// Build objects array (only OID and Size, no path/sample)
	objects := make([]LFSBatchObject, len(files))
	for i, file := range files {
		objects[i] = LFSBatchObject{
			OID:  file.OID,
			Size: file.Size,
		}
	}

	// Create LFS batch request with required fields
	payload := LFSBatchRequest{
		Operation: "upload",
		Transfers: []string{"basic", "multipart"},
		Objects:   objects,
		HashAlgo:  "sha256",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}

	// Set LFS-specific headers
	req.Header.Set("Authorization", "Bearer "+u.token)
	req.Header.Set("Content-Type", "application/vnd.git-lfs+json")
	req.Header.Set("Accept", "application/vnd.git-lfs+json")

	u.logger.Debug("LFS batch request", "url", url, "file_count", len(files))

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
		return nil, fmt.Errorf("LFS batch failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var batchResp LFSBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		return nil, fmt.Errorf("failed to decode LFS batch response: %w", err)
	}

	// Parse response and create upload map
	uploadMap := make(map[string]*LFSUploadInfo)
	for _, obj := range batchResp.Objects {
		info := &LFSUploadInfo{
			OID:  obj.OID,
			Size: obj.Size,
		}

		// If actions is nil, file already exists on server (this is NORMAL)
		if obj.Actions != nil && obj.Actions.Upload != nil {
			info.UploadURL = obj.Actions.Upload.Href
			info.Header = obj.Actions.Upload.Header
		}

		uploadMap[obj.OID] = info
	}

	u.logger.Info("LFS batch completed", "objects", len(uploadMap), "transfer", batchResp.Transfer)
	return uploadMap, nil
}

// UploadLFSFile uploads a file using either basic or multipart LFS protocol
func (u *Uploader) UploadLFSFile(uploadInfo *LFSUploadInfo, filePath string) error {
	if uploadInfo.UploadURL == "" {
		// File already exists on server, no upload needed
		u.logger.Debug("LFS file already exists on server", "oid", uploadInfo.OID)
		return nil
	}

	// Check if this is multipart upload by looking for chunk_size in header
	chunkSizeStr, hasChunkSize := uploadInfo.Header["chunk_size"]
	if hasChunkSize {
		// Multipart upload
		return u.uploadLFSFileMultipart(uploadInfo, filePath, chunkSizeStr)
	}

	// Basic single-part upload
	return u.uploadLFSFileBasic(uploadInfo, filePath)
}

// uploadLFSFileBasic uploads a file using basic LFS protocol (single PUT)
func (u *Uploader) uploadLFSFileBasic(uploadInfo *LFSUploadInfo, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	u.logger.Debug("Uploading LFS file (basic)", "oid", uploadInfo.OID, "size", fileInfo.Size())

	req, err := http.NewRequest("PUT", uploadInfo.UploadURL, file)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = fileInfo.Size()

	// Add any additional headers (excluding chunk_size and part URLs)
	for key, value := range uploadInfo.Header {
		if key != "chunk_size" && !isNumericKey(key) {
			req.Header.Set(key, value)
		}
	}

	resp, err := u.lfsClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("LFS upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	u.logger.Info("LFS file uploaded (basic)", "oid", uploadInfo.OID, "size", fileInfo.Size())
	return nil
}

// uploadLFSFileMultipart uploads a file using multipart LFS protocol
func (u *Uploader) uploadLFSFileMultipart(uploadInfo *LFSUploadInfo, filePath string, chunkSizeStr string) error {
	chunkSize := int64(0)
	if _, err := fmt.Sscanf(chunkSizeStr, "%d", &chunkSize); err != nil || chunkSize <= 0 {
		return fmt.Errorf("invalid chunk_size: %s", chunkSizeStr)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	// Extract part URLs from header (keys "1", "2", "3", ...)
	partURLs := extractPartURLs(uploadInfo.Header)
	if len(partURLs) == 0 {
		return fmt.Errorf("no part URLs found in multipart upload response")
	}

	u.logger.Debug("Uploading LFS file (multipart)",
		"oid", uploadInfo.OID,
		"size", fileInfo.Size(),
		"chunk_size", chunkSize,
		"parts", len(partURLs))

	// Upload each part and collect ETags
	type partResult struct {
		partNumber int
		etag       string
	}
	results := make([]partResult, 0, len(partURLs))

	for partNum, partURL := range partURLs {
		// Calculate offset and length for this part
		offset := int64(partNum-1) * chunkSize
		length := chunkSize
		if offset+length > fileInfo.Size() {
			length = fileInfo.Size() - offset
		}

		// Seek to start of this part
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek to offset %d: %w", offset, err)
		}

		// Upload this part
		limitedReader := io.LimitReader(file, length)
		req, err := http.NewRequest("PUT", partURL, limitedReader)
		if err != nil {
			return fmt.Errorf("failed to create request for part %d: %w", partNum, err)
		}

		req.Header.Set("Content-Type", "application/octet-stream")
		req.ContentLength = length

		resp, err := u.lfsClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to upload part %d: %w", partNum, err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return fmt.Errorf("part %d upload failed with status %d: %s", partNum, resp.StatusCode, string(bodyBytes))
		}

		// Extract ETag from response
		etag := resp.Header.Get("ETag")
		_ = resp.Body.Close()

		if etag == "" {
			return fmt.Errorf("no ETag returned for part %d", partNum)
		}

		results = append(results, partResult{partNumber: partNum, etag: etag})
		u.logger.Debug("Uploaded part", "part", partNum, "etag", etag)
	}

	// Sort results by part number (S3 requires ascending order)
	sort.Slice(results, func(i, j int) bool {
		return results[i].partNumber < results[j].partNumber
	})

	// Send completion request
	completionPayload := map[string]interface{}{
		"oid": uploadInfo.OID,
		"parts": func() []map[string]interface{} {
			parts := make([]map[string]interface{}, len(results))
			for i, r := range results {
				parts[i] = map[string]interface{}{
					"partNumber": r.partNumber,
					"etag":       r.etag,
				}
			}
			return parts
		}(),
	}

	completionJSON, err := json.Marshal(completionPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal completion payload: %w", err)
	}

	req, err := http.NewRequest("POST", uploadInfo.UploadURL, bytes.NewReader(completionJSON))
	if err != nil {
		return fmt.Errorf("failed to create completion request: %w", err)
	}

	req.Header.Set("Content-Type", "application/vnd.git-lfs+json")
	req.Header.Set("Accept", "application/vnd.git-lfs+json")

	resp, err := u.lfsClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send completion request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("completion request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	u.logger.Info("LFS file uploaded (multipart)", "oid", uploadInfo.OID, "size", fileInfo.Size(), "parts", len(partURLs))
	return nil
}

// extractPartURLs extracts part URLs from header map (keys like "1", "2", "3"...)
func extractPartURLs(header map[string]string) map[int]string {
	partURLs := make(map[int]string)
	for key, value := range header {
		if isNumericKey(key) {
			partNum := 0
			if _, err := fmt.Sscanf(key, "%d", &partNum); err == nil && partNum > 0 {
				partURLs[partNum] = value
			}
		}
	}
	return partURLs
}

// isNumericKey checks if a string is a numeric key (used for multipart part numbers)
func isNumericKey(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
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
