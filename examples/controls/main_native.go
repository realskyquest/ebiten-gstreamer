//go:build !js

package main

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/realskyquest/ebiten-gstreamer/video"
)

// loadVideoFromFS copies the dropped file to a temp path on disk and plays it.
// GStreamer requires a real filesystem path, so a temp file is necessary.
func (g *Game) loadVideoFromFS(droppedFS fs.FS, name string) {
	src, err := droppedFS.Open(name)
	if err != nil {
		log.Printf("Failed to open dropped file: %v", err)
		g.showToast("Error opening: " + name)
		return
	}
	defer src.Close()

	ext := filepath.Ext(name)
	tmpFile, err := os.CreateTemp("", "ebiten-video-*"+ext)
	if err != nil {
		log.Printf("Failed to create temp file: %v", err)
		g.showToast("Error: could not create temp file")
		return
	}

	if _, err := io.Copy(tmpFile, src); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		log.Printf("Failed to copy dropped file: %v", err)
		g.showToast("Error: could not read dropped file")
		return
	}
	tmpFile.Close()

	// Remove the previous temp file only after the new one is fully written.
	g.cleanupTemp()

	g.tempFile = tmpFile.Name()
	g.loadVideo(g.tempFile)
}

// cleanupTemp removes the active temp file if one exists.
func (g *Game) cleanupTemp() {
	if g.tempFile != "" {
		os.Remove(g.tempFile)
		g.tempFile = ""
	}
}

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
	game.cleanupTemp()
}
