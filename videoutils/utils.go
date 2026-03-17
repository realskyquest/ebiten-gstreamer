package videoutils

import (
	"fmt"

	"github.com/realskyquest/ebiten-gstreamer/video"
)

// VideoExts is the set of file extensions accepted via drag-and-drop.
var VideoExts = map[string]bool{
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

// LoadVideo closes the current player (if any) and opens a new video from source.
func LoadVideo(videoCtx *video.Context, player *video.Player, source string, opts *video.PlayerOptions) (*video.Player, string, error) {
	if player != nil {
		player.Close()
		player = nil
	}

	newPlayer, err := videoCtx.NewPlayer(source, opts)
	if err != nil {
		return nil, fmt.Sprintf("Error: %s", err), fmt.Errorf("videoutils: failed to load video: %w", err)
	}

	display := source
	if len(display) > 50 {
		display = "..." + display[len(display)-47:]
	}
	return newPlayer, fmt.Sprintf("Loaded: %s", display), nil
}
