//go:build !js

package videoutils

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// loadVideoFromFS copies the dropped file to a temp path on disk and returns
// the path. GStreamer requires a real filesystem path, so a temp file is necessary.
// Returns the temp file path, message, and error.
func loadVideoFromFile(droppedFS fs.FS, name string) (string, string, error) {
	src, err := droppedFS.Open(name)
	if err != nil {
		return "", "Error opening: " + name, fmt.Errorf("%w: %w", ErrOpenFailed, err)
	}
	defer src.Close()

	ext := filepath.Ext(name)
	tmpFile, err := os.CreateTemp("", "ebiten-video-*"+ext)
	if err != nil {
		return "", "Error: could not create temp file", fmt.Errorf("%w: %w", ErrTempFile, err)
	}

	if _, err := io.Copy(tmpFile, src); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", "Error: could not read dropped file", fmt.Errorf("%w: %w", ErrCopyFailed, err)
	}
	tmpFile.Close()

	return tmpFile.Name(), "", nil
}

// CleanupTempFile removes the file if it exists.
func CleanupTempFile(name string) {
	if name != "" {
		_ = os.Remove(name)
	}
}
