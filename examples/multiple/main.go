//go:build !js

package main

import (
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/realskyquest/ebiten-gstreamer/video"
	"github.com/realskyquest/ebiten-gstreamer/videoutils"
)

func main() {
	ctx, err := video.NewContext()
	if err != nil {
		log.Fatal(err)
	}
	defer ctx.Close()

	game := &Game{
		videoCtx:   ctx,
		dropTarget: 0,
	}

	// Optionally pre-load up to two sources from command-line arguments.
	for i := 0; i < 2 && i+1 < len(os.Args); i++ {
		game.slots[i].player, err = game.videoCtx.NewPlayer(os.Args[i+1], &video.PlayerOptions{
			Volume: 0.8,
			OnEnd: func() {
				log.Printf("Slot %d: video ended", i)
			},
			OnError: func(err error) {
				log.Printf("Slot %d: pipeline error: %v", i, err)
			},
		})
		game.slots[i].player.Play()
	}

	ebiten.SetWindowTitle("Video Player — Two Slots")

	if err := runGame(game); err != nil {
		log.Fatal(err)
	}

	// Clean up any remaining temp files.
	for i := range game.slots {
		videoutils.CleanupTempFile(game.slots[i].tempFile)
	}
}
