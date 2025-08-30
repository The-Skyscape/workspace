package worker

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	
	"github.com/pkg/errors"
)

// Embedded worker binary
var (
	//go:embed resources/worker
	workerBinary []byte
)

// ExtractWorkerBinary writes the embedded worker binary to a temporary file
// and returns the path to the extracted binary. The caller is responsible
// for cleaning up the file when done.
func ExtractWorkerBinary() (string, error) {
	// Create temp directory for worker binary
	tempDir, err := os.MkdirTemp("", "skyscape-worker-*")
	if err != nil {
		return "", errors.Wrap(err, "failed to create temp directory")
	}
	
	// Path for the worker binary
	workerPath := filepath.Join(tempDir, "worker")
	
	// Write the embedded binary to temp file
	if err := os.WriteFile(workerPath, workerBinary, 0755); err != nil {
		os.RemoveAll(tempDir)
		return "", errors.Wrap(err, "failed to write worker binary")
	}
	
	return workerPath, nil
}

// GetWorkerBinarySize returns the size of the embedded worker binary
func GetWorkerBinarySize() int {
	return len(workerBinary)
}

// IsWorkerEmbedded checks if the worker binary is embedded
func IsWorkerEmbedded() bool {
	return len(workerBinary) > 0
}

// CleanupWorkerBinary removes the temporary directory containing the worker binary
func CleanupWorkerBinary(workerPath string) error {
	if workerPath == "" {
		return nil
	}
	
	// Get the parent directory (temp directory we created)
	tempDir := filepath.Dir(workerPath)
	
	// Only remove if it looks like our temp directory
	if filepath.Base(tempDir)[:15] == "skyscape-worker" {
		return os.RemoveAll(tempDir)
	}
	
	return fmt.Errorf("refusing to remove non-temporary directory: %s", tempDir)
}