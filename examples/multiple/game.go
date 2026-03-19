package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/realskyquest/ebiten-gstreamer/video"
	"github.com/realskyquest/ebiten-gstreamer/videoutils"
)

// pixel is a cached 1×1 white image used by drawRect.
var pixel *ebiten.Image

func init() {
	pixel = ebiten.NewImage(1, 1)
	pixel.Fill(color.White)
}

// slot holds a single video player and its associated temp file.
type slot struct {
	player   *video.Player
	tempFile string
}

type Game struct {
	videoCtx *video.Context
	slots    [2]slot

	// dropTarget is the slot index (0 or 1) that the next dropped file goes to.
	dropTarget int
}

func (g *Game) Update() error {
	// Handle drag-and-drop — each dropped file goes to the next slot in turn.
	if droppedFS := ebiten.DroppedFiles(); droppedFS != nil {
		player, tempFile, msg, err := videoutils.LoadVideoFromFS(droppedFS, g.videoCtx, g.slots[g.dropTarget].player, g.slots[g.dropTarget].tempFile, &video.PlayerOptions{
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
		g.slots[g.dropTarget].player = player
		g.slots[g.dropTarget].tempFile = tempFile
		player.Play()
		g.dropTarget = 1 - g.dropTarget
	}

	// Space — toggle play/pause on both slots.
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		for i := range g.slots {
			p := g.slots[i].player
			if p == nil {
				continue
			}
			if p.IsPlaying() {
				p.Pause()
			} else {
				p.Play()
			}
		}
	}

	// Quit
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		return ebiten.Termination
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)

	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Each slot occupies half the window height.
	slotH := sh / 2

	for i, s := range g.slots {
		offsetY := i * slotH

		if s.player == nil {
			// Prompt for the empty slot.
			var msg string
			if i == g.dropTarget {
				msg = fmt.Sprintf("Drop a video file here (slot %d) ↓", i+1)
			} else {
				msg = fmt.Sprintf("Slot %d — drop a file or pass an argument", i+1)
			}
			msgX := (sw - len(msg)*6) / 2
			ebitenutil.DebugPrintAt(screen, msg, msgX, offsetY+slotH/2-8)
			continue
		}

		p := s.player

		// Draw the frame letterboxed within the slot's rectangle.
		if frame := p.Frame(); frame != nil {
			vw, vh := p.VideoSize()
			if vw > 0 && vh > 0 {
				scaleX := float64(sw) / float64(vw)
				scaleY := float64(slotH) / float64(vh)
				scale := math.Min(scaleX, scaleY)

				opts := &ebiten.DrawImageOptions{}
				opts.Filter = ebiten.FilterLinear
				opts.GeoM.Scale(scale, scale)
				opts.GeoM.Translate(
					(float64(sw)-float64(vw)*scale)/2,
					float64(offsetY)+(float64(slotH)-float64(vh)*scale)/2,
				)
				screen.DrawImage(frame, opts)
			}
		}

		// HUD for this slot.
		pos := p.Position()
		dur := p.Duration()

		state := "▶"
		if !p.IsPlaying() {
			state = "⏸"
		}
		if p.IsEOS() {
			state = "⏹"
		}

		hud := fmt.Sprintf("Slot %d  %s  %s / %s", i+1, state, fmtDur(pos), fmtDur(dur))
		ebitenutil.DebugPrintAt(screen, hud, 8, offsetY+4)

		// Progress bar along the bottom of the slot.
		if dur > 0 {
			barMargin := 20.0
			barW := float64(sw) - barMargin*2
			barH := 4.0
			barY := float64(offsetY+slotH) - barH - 4

			drawRect(screen, barMargin, barY, barW, barH, color.RGBA{80, 80, 80, 180})

			progress := float64(pos) / float64(dur)
			if progress > 1 {
				progress = 1
			}
			if fillW := barW * progress; fillW >= 1 {
				drawRect(screen, barMargin, barY, fillW, barH, color.RGBA{50, 200, 80, 220})
			}
		}
	}

	// Divider line between the two slots.
	drawRect(screen, 0, float64(slotH)-1, float64(sw), 2, color.RGBA{60, 60, 60, 255})

	// Global controls hint.
	hint := "Space: Play/Pause both  |  Drop: load into next slot  |  Q/Esc: Quit"
	hintX := (sw - len(hint)*6) / 2
	if hintX < 4 {
		hintX = 4
	}
	ebitenutil.DebugPrintAt(screen, hint, hintX, sh-14)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func drawRect(screen *ebiten.Image, x, y, w, h float64, clr color.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(w, h)
	opts.GeoM.Translate(x, y)
	opts.ColorScale.ScaleWithColor(clr)
	screen.DrawImage(pixel, opts)
}

func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Truncate(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// runGame configures and starts the Ebiten game loop.
func runGame(game *Game) error {
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	return ebiten.RunGame(game)
}
