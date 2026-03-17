//go:build !js

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/realskyquest/ebiten-gstreamer/video"
	"github.com/realskyquest/ebiten-gstreamer/videoutils"
)

func clampVol(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1.5 {
		return 1.5
	}
	return v
}

func main() {
	ctx, err := video.NewContext()
	if err != nil {
		log.Fatal(err)
	}
	defer ctx.Close()

	game := &Game{
		videoCtx: ctx,
		savedVol: 0.8,
	}

	// Load a file or URL passed as the first command-line argument, if any.
	if len(os.Args) >= 2 {
		source := os.Args[1]
		player, err := ctx.NewPlayer(source, &video.PlayerOptions{
			Volume: 0.8,
			OnEnd: func() {
				log.Println("Video ended")
			},
			OnError: func(err error) {
				log.Println("Pipeline error:", err)
			},
		})
		if err != nil {
			log.Printf("Failed to load %s: %v", source, err)
		} else {
			game.player = player
			player.Play()
			ebiten.SetWindowTitle(fmt.Sprintf("Video Player — %s", filepath.Base(source)))
		}
	} else {
		ebiten.SetWindowTitle("Video Player — Drop a file to play")
	}

	if err := runGame(game); err != nil {
		log.Fatal(err)
	}

	// Clean up any drag-and-drop temp file left over at exit.
	videoutils.CleanupTempFile(game.tempFile)
}
