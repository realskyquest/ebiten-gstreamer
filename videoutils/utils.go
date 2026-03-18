package videoutils

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/realskyquest/ebiten-gstreamer/video"
)

// VideoExts is the set of file extensions accepted via drag-and-drop.
var videoExts = map[string]bool{
	".mp4":  true,
	".mkv":  true,
	".avi":  true,
	".webm": true,
	".mov":  true,
	".flv":  true,
	".wmv":  true,
	".m4v":  true,
	".ts":   true,
	".ogv":  true,
	".3gp":  true,
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

// loadVideo closes the current player (if any) and opens a new video from source.
// Returns the new player, message, and error.
func loadVideo(videoCtx *video.Context, player *video.Player, source string, opts *video.PlayerOptions) (*video.Player, string, error) {
	if player != nil {
		player.Close()
		player = nil
	}

	newPlayer, err := videoCtx.NewPlayer(source, opts)
	if err != nil {
		return nil, fmt.Sprintf("Error: %s", err), fmt.Errorf("%w: %w", ErrLoadVideoFailed, err)
	}

	display := source
	if len(display) > 50 {
		display = "..." + display[len(display)-47:]
	}
	return newPlayer, fmt.Sprintf("Loaded: %s", display), nil
}

// LoadVideoFromFS gets the video file from the dropped file system.
// Returns the new player, temp file path or blob URL, message, and error.
func LoadVideoFromFS(droppedFS fs.FS, videoCtx *video.Context, player *video.Player, tempFile string, opts *video.PlayerOptions) (*video.Player, string, string, error) {
	var p *video.Player
	var t, m string

	entries, err := fs.ReadDir(droppedFS, ".")
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if videoExts[ext] {
				newTempFile, msg, err := loadVideoFromFile(droppedFS, entry.Name())
				if err != nil {
					return nil, "", msg, err
				}

				// Remove the previous temp file.
				CleanupTempFile(tempFile)

				t = newTempFile

				newPlayer, msg, err := loadVideo(videoCtx, player, t, opts)
				if err != nil {
					return nil, "", msg, err
				}
				p = newPlayer
				m = msg

				break
			}
		}
	}

	return p, t, m, nil
}
