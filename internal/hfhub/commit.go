package hfhub

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// CommitOperation represents a single operation in a commit
type CommitOperation struct {
	Operation string                 `json:"operation"` // "add", "delete", "copy", "move"
	Path      string                 `json:"path"`
	Content   string                 `json:"content,omitempty"`   // base64 encoded for small files
	Encoding  string                 `json:"encoding,omitempty"`  // "base64" for content
	OldPath   string                 `json:"oldPath,omitempty"`   // for copy/move
	LFSFile   *LFSFileInfo           `json:"lfsFile,omitempty"`   // for large files
	ExtraInfo map[string]interface{} `json:"extraInfo,omitempty"` // additional metadata
}

// LFSFileInfo contains information about an LFS file
type LFSFileInfo struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// LFSThreshold is the size threshold for using LFS (10MB)
const LFSThreshold = 10 * 1024 * 1024

// PrepareFileOperation prepares a commit operation for a file
func PrepareFileOperation(localPath, pathInRepo string) (*CommitOperation, error) {
	// Get file info
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, err
	}

	// Read file
	file, err := os.Open(localPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			// File close errors are critical for data integrity
			// Log but don't fail since file was already read
			fmt.Fprintf(os.Stderr, "Warning: failed to close file %s: %v\n", localPath, err)
		}
	}()

	// Calculate SHA256
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return nil, err
	}
	sha := hex.EncodeToString(hasher.Sum(nil))

	op := &CommitOperation{
		Operation: "add",
		Path:      pathInRepo,
	}

	// Small files: embed content as base64
	if info.Size() < LFSThreshold {
		if _, err := file.Seek(0, 0); err != nil {
			return nil, err
		}
		data, err := io.ReadAll(file)
		if err != nil {
			return nil, err
		}

		op.Content = base64.StdEncoding.EncodeToString(data)
		op.Encoding = "base64"
	} else {
		// Large files: use LFS
		op.LFSFile = &LFSFileInfo{
			SHA256: sha,
			Size:   size,
		}
	}

	return op, nil
}
