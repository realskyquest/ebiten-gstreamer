//go:build js && wasm

package videoutils

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"syscall/js"
)

// mimeForExt returns a best-effort MIME type for common video extensions.
func mimeForExt(ext string) string {
	switch ext {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".ogv":
		return "video/ogg"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	case ".ts":
		return "video/mp2t"
	case ".flv":
		return "video/x-flv"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".3gp":
		return "video/3gpp"
	default:
		return "video/mp4"
	}
}

// loadVideoFromFS reads the dropped file into a JS Blob and plays it via a
// blob URL. No temp file is needed because the HTML5 player accepts a URL
// directly, and os.CreateTemp is not available on Wasm.
func LoadVideoFromFS(droppedFS fs.FS, name string) (string, string, error) {
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

// CleanupTempFile revokes the active blob URL, releasing the browser's reference
// to the underlying file data.
func CleanupTempFile(name string) {
	if name != "" {
		_ = js.Global().Get("URL").Call("revokeObjectURL", name)
	}
}
