package main

import (
	"fmt"
	"log"
	"math"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/realskyquest/ebiten-gstreamer/video"
	"github.com/realskyquest/ebiten-gstreamer/videoutils"
)

type Game struct {
	videoCtx *video.Context
	player   *video.Player

	tempFile string
}

func (g *Game) Update() error {
	// Handle drag-and-drop.
	if droppedFS := ebiten.DroppedFiles(); droppedFS != nil {
		player, tempFile, msg, err := videoutils.LoadVideoFromFS(droppedFS, g.videoCtx, g.player, g.tempFile, &video.PlayerOptions{
			Volume: 1.0,
			OnEnd: func() {
				log.Println("Video ended")
			},
			OnError: func(err error) {
				log.Println("Pipeline error:", err)
			},
		})
		log.Println(msg)
		if err != nil {
			log.Println(err)
			return nil
		}
		g.player = player
		g.tempFile = tempFile
		player.Play()
		ebiten.SetWindowTitle(fmt.Sprintf("Video Player — %s", filepath.Base(g.tempFile)))
	}

	p := g.player
	if p == nil {
		return nil
	}

	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		if p.IsPlaying() {
			p.Pause()
		} else {
			p.Play()
		}
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	p := g.player

	if p == nil {
		msg := "Drag and drop a video file here to play"
		ebitenutil.DebugPrintAt(screen, msg, (sw-len(msg)*6)/2, sh/2-8)
		return
	}

	// Draw video frame, letterboxed inside the window.
	if frame := p.Frame(); frame != nil {
		vw, vh := p.VideoSize()
		if vw > 0 && vh > 0 {
			scaleX := float64(sw) / float64(vw)
			scaleY := float64(sh) / float64(vh)
			scale := math.Min(scaleX, scaleY)

			opts := &ebiten.DrawImageOptions{}
			opts.Filter = ebiten.FilterLinear
			opts.GeoM.Scale(scale, scale)
			opts.GeoM.Translate(
				(float64(sw)-float64(vw)*scale)/2,
				(float64(sh)-float64(vh)*scale)/2,
			)
			screen.DrawImage(frame, opts)
		}
	}

	ebitenutil.DebugPrint(screen,
		fmt.Sprintf("Position: %v / %v", g.player.Position(), g.player.Duration()))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

// runGame configures and starts the Ebiten game loop.
func runGame(game *Game) error {
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	return ebiten.RunGame(game)
}
