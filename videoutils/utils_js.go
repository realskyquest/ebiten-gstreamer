//go:build js && wasm

package videoutils

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"syscall/js"
)

// loadVideoFromFile reads the dropped file into a JS Blob and plays it via a blob URL.
// Reads video file from the virutal filesystem using chunking.
// Returns the blob URL, message, and error.
func loadVideoFromFile(droppedFS fs.FS, name string) (string, string, error) {
	src, err := droppedFS.Open(name)
	if err != nil {
		return "", "Error opening: " + name, fmt.Errorf("%w: %w", ErrOpenFailed, err)
	}
	defer src.Close()

	const chunkSize = 5 * 1024 * 1024 // 5 MB chunks
	blobParts := js.Global().Get("Array").New()
	buffer := make([]byte, chunkSize)
	index := 0

	for {
		n, err := src.Read(buffer)
		if n > 0 {
			jsData := js.Global().Get("Uint8Array").New(n)
			js.CopyBytesToJS(jsData, buffer[:n])
			blobParts.Call("push", jsData)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "Error reading file", fmt.Errorf("%w: %w", ErrCopyFailed, err)
		}
		index++
	}

	blob := js.Global().Get("Blob").New(blobParts, map[string]interface{}{
		"type": mimeForExt(filepath.Ext(name)),
	})
	blobURL := js.Global().Get("URL").Call("createObjectURL", blob).String()

	return blobURL, "", nil
}

// CleanupTempFile revokes the blob URL, releasing the browser's reference
// to the underlying file data.
func CleanupTempFile(name string) {
	if name != "" {
		_ = js.Global().Get("URL").Call("revokeObjectURL", name)
	}
}
