//go:build js && wasm

package main

import (
	"io"
	"io/fs"
	"log"
	"path/filepath"
	"syscall/js"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/realskyquest/ebiten-gstreamer/video"
)

// loadVideoFromFS reads the dropped file into a JS Blob and plays it via a
// blob URL. No temp file is needed because the HTML5 player accepts a URL
// directly, and os.CreateTemp is not available on Wasm.
func (g *Game) loadVideoFromFS(droppedFS fs.FS, name string) {
	src, err := droppedFS.Open(name)
	if err != nil {
		log.Printf("Failed to open dropped file: %v", err)
		g.showToast("Error opening: " + name)
		return
	}
	defer src.Close()

	data, err := io.ReadAll(src)
	if err != nil {
		log.Printf("Failed to read dropped file: %v", err)
		g.showToast("Error: could not read dropped file")
		return
	}

	// Copy bytes into a JS Uint8Array, wrap in a Blob, then create an object URL.
	jsData := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(jsData, data)

	blobParts := js.Global().Get("Array").New(1)
	blobParts.SetIndex(0, jsData)
	blob := js.Global().Get("Blob").New(blobParts, map[string]interface{}{
		"type": mimeForExt(filepath.Ext(name)),
	})
	blobURL := js.Global().Get("URL").Call("createObjectURL", blob).String()

	// Revoke the previous blob URL before replacing it.
	g.cleanupTemp()

	g.tempFile = blobURL
	g.loadVideo(blobURL)
}

// cleanupTemp revokes the active blob URL, releasing the browser's reference
// to the underlying file data.
func (g *Game) cleanupTemp() {
	if g.tempFile != "" {
		js.Global().Get("URL").Call("revokeObjectURL", g.tempFile)
		g.tempFile = ""
	}
}

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

func clampVol(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func main() {
	ctx, err := video.NewContext()
	if err != nil {
		log.Fatal(err)
	}
	// Context is not closed via defer: on the web RunGame never returns normally
	// and page lifetime is managed by the browser.

	game := &Game{
		videoCtx: ctx,
		savedVol: 0.8,
	}

	ebiten.SetWindowTitle("Video Player — Drop a file to play")

	if err := runGame(game); err != nil {
		log.Fatal(err)
	}
}
