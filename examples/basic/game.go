package main

import (
	"fmt"
	"io/fs"
	"log"
	"math"
	"path/filepath"
	"strings"

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

func (g *Game) loadVideo(droppedFS fs.FS, entry fs.DirEntry) {
	tempFile, msg, err := videoutils.LoadVideoFromFS(droppedFS, entry.Name())
	if err != nil {
		log.Println(msg)
		log.Println(err)
		return
	}
	// Remove the previous temp file.
	videoutils.CleanupTempFile(g.tempFile)
	g.tempFile = tempFile
	newPlayer, msg, err := videoutils.LoadVideo(g.videoCtx, g.player, g.tempFile, &video.PlayerOptions{
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
		return
	}
	g.player = newPlayer
	g.player.Play()
	ebiten.SetWindowTitle(fmt.Sprintf("Video Player — %s", filepath.Base(g.tempFile)))
}

func (g *Game) Update() error {
	// Handle drag-and-drop.
	if droppedFS := ebiten.DroppedFiles(); droppedFS != nil {
		entries, err := fs.ReadDir(droppedFS, ".")
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				ext := strings.ToLower(filepath.Ext(entry.Name()))
				if videoutils.VideoExts[ext] {
					g.loadVideo(droppedFS, entry)
					break
				}
			}
		}
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
