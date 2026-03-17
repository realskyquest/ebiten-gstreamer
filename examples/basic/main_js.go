//go:build js && wasm

package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/realskyquest/ebiten-gstreamer/video"
)

func main() {
	ctx, err := video.NewContext()
	if err != nil {
		log.Fatal(err)
	}
	// Context is not closed via defer: on the web RunGame never returns normally
	// and page lifetime is managed by the browser.

	game := &Game{
		videoCtx: ctx,
	}

	ebiten.SetWindowTitle("Video Player — Drop a file to play")

	if err := runGame(game); err != nil {
		log.Fatal(err)
	}
}
