package main

import (
	"fmt"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/realskyquest/ebiten-gstreamer/video"
)

type Game struct {
	player *video.Player
}

func (g *Game) Update() error {
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	frame := g.player.Frame()
	if frame == nil {
		return
	}

	// Scale video to fit window
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	vw, vh := g.player.VideoSize()
	if vw == 0 || vh == 0 {
		return
	}

	scaleX := float64(sw) / float64(vw)
	scaleY := float64(sh) / float64(vh)
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate(
		(float64(sw)-float64(vw)*scale)/2,
		(float64(sh)-float64(vh)*scale)/2,
	)
	screen.DrawImage(frame, opts)

	ebitenutil.DebugPrint(screen,
		fmt.Sprintf("Position: %v / %v", g.player.Position(), g.player.Duration()))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: basic <file-or-url>")
	}
	source := os.Args[1]

	ctx, err := video.NewContext()
	if err != nil {
		log.Fatal(err)
	}
	defer ctx.Close()

	player, err := ctx.NewPlayer(source, &video.PlayerOptions{
		Volume: 0.8,
		OnEnd: func() {
			fmt.Println("Video ended!")
		},
		OnError: func(err error) {
			fmt.Println("Error:", err)
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	player.Play()

	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("Ebitengine GStreamer Video Player")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(&Game{player: player}); err != nil {
		log.Fatal(err)
	}
}
