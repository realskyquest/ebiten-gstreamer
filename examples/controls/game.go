package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/realskyquest/ebiten-gstreamer/video"
	"github.com/realskyquest/ebiten-gstreamer/videoutils"
)

const (
	seekStep      = 5 * time.Second
	seekStepLarge = 30 * time.Second
	volumeStep    = 0.05
)

// rateKeys maps number keys 1–9 to playback rates.
var rateKeys = []struct {
	key  ebiten.Key
	rate float64
}{
	{ebiten.Key1, 0.25},
	{ebiten.Key2, 0.5},
	{ebiten.Key3, 0.75},
	{ebiten.Key4, 1.0},
	{ebiten.Key5, 1.25},
	{ebiten.Key6, 1.5},
	{ebiten.Key7, 1.75},
	{ebiten.Key8, 2.0},
	{ebiten.Key9, 4.0},
}

// pixel is a cached 1×1 white image used by drawRect.
var pixel *ebiten.Image

func init() {
	pixel = ebiten.NewImage(1, 1)
	pixel.Fill(color.White)
}

type Game struct {
	videoCtx *video.Context
	player   *video.Player
	muted    bool
	savedVol float64

	toast      string
	toastUntil time.Time

	// tempFile tracks any drag-and-drop resource (temp path or blob URL) so it
	// can be released when a new file is loaded or the app exits.
	tempFile string
}

func (g *Game) showToast(msg string) {
	g.toast = msg
	g.toastUntil = time.Now().Add(1500 * time.Millisecond)
}

func (g *Game) Update() error {
	// Handle drag-and-drop.
	if droppedFS := ebiten.DroppedFiles(); droppedFS != nil {
		player, tempFile, msg, err := videoutils.LoadVideoFromFS(droppedFS, g.videoCtx, g.player, g.tempFile, &video.PlayerOptions{
			Volume: 8.0,
			OnEnd: func() {
				log.Println("Video ended")
			},
			OnError: func(err error) {
				log.Println("Pipeline error:", err)
			},
		})
		g.showToast(msg)
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

	// Play / Pause
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		if p.IsPlaying() {
			p.Pause()
			g.showToast("⏸  Paused")
		} else {
			p.Play()
			g.showToast("▶  Playing")
		}
	}

	// Restart
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		if err := p.Rewind(); err != nil {
			g.showToast(fmt.Sprintf("rewind failed: %s", err))
			return nil
		}
		if !p.IsPlaying() {
			p.Play()
		}
		g.showToast("⏮  Restarted")
	}

	// Seek forward
	if inpututil.IsKeyJustPressed(ebiten.KeyRight) {
		step := seekStep
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			step = seekStepLarge
		}
		target := p.Position() + step
		if dur := p.Duration(); dur > 0 && target > dur {
			target = dur
		}
		if err := p.SetPosition(target); err != nil {
			g.showToast(fmt.Sprintf("seek failed: %s", err))
			return nil
		}
		g.showToast(fmt.Sprintf("+%s", fmtDur(step)))
	}

	// Seek backward
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) {
		step := seekStep
		if ebiten.IsKeyPressed(ebiten.KeyShift) {
			step = seekStepLarge
		}
		target := p.Position() - step
		if target < 0 {
			target = 0
		}
		if err := p.SetPosition(target); err != nil {
			g.showToast(fmt.Sprintf("seek failed: %s", err))
			return nil
		}
		g.showToast(fmt.Sprintf("-%s", fmtDur(step)))
	}

	// Volume up
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		if g.muted {
			g.muted = false
		}
		vol := clampVol(p.Volume() + volumeStep)
		p.SetVolume(vol)
		g.showToast(fmt.Sprintf("Volume %d%%", int(math.Round(vol*100))))
	}

	// Volume down
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		if g.muted {
			g.muted = false
		}
		vol := clampVol(p.Volume() - volumeStep)
		p.SetVolume(vol)
		g.showToast(fmt.Sprintf("Volume %d%%", int(math.Round(vol*100))))
	}

	// Mute / Unmute
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		if g.muted {
			p.SetVolume(g.savedVol)
			g.muted = false
			g.showToast(fmt.Sprintf("Unmuted (%d%%)", int(math.Round(g.savedVol*100))))
		} else {
			g.savedVol = p.Volume()
			p.SetVolume(0)
			g.muted = true
			g.showToast("Muted")
		}
	}

	// Loop toggle
	if inpututil.IsKeyJustPressed(ebiten.KeyL) {
		p.SetLoop(!p.Loop())
		if p.Loop() {
			g.showToast("Loop ON")
		} else {
			g.showToast("Loop OFF")
		}
	}

	// Playback rate — keys 1–9 map to increasing speeds
	for _, rk := range rateKeys {
		if inpututil.IsKeyJustPressed(rk.key) {
			p.SetRate(rk.rate)
			g.showToast(fmt.Sprintf("Speed %.2gx", rk.rate))
			break
		}
	}

	// Quit
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		return ebiten.Termination
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	p := g.player

	if p == nil {
		msg := "Drag and drop a video file here to play"
		ebitenutil.DebugPrintAt(screen, msg, (sw-len(msg)*6)/2, sh/2-8)
		g.drawControlsHelp(screen, sw, sh)
		g.drawToast(screen, sw, sh)
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

	// HUD: state / position / volume / loop / speed
	pos := p.Position()
	dur := p.Duration()
	vol := p.Volume()
	if g.muted {
		vol = 0
	}

	state := "▶ Playing"
	if !p.IsPlaying() {
		state = "⏸ Paused"
	}
	if p.IsEOS() {
		state = "⏹ Ended"
	}

	loopStr := "OFF"
	if p.Loop() {
		loopStr = "ON"
	}

	info := fmt.Sprintf(
		"%s  |  %s / %s  |  Vol: %d%%  |  Loop: %s  |  Speed: %.2gx",
		state,
		fmtDur(pos), fmtDur(dur),
		int(math.Round(vol*100)),
		loopStr,
		p.Rate(),
	)
	ebitenutil.DebugPrint(screen, info)

	// Progress bar
	if dur > 0 {
		barY := float64(sh) - 30
		barMargin := 20.0
		barW := float64(sw) - barMargin*2
		barH := 6.0

		drawRect(screen, barMargin, barY, barW, barH, color.RGBA{80, 80, 80, 180})

		progress := float64(pos) / float64(dur)
		if progress > 1 {
			progress = 1
		}
		if fillW := barW * progress; fillW >= 1 {
			drawRect(screen, barMargin, barY, fillW, barH, color.RGBA{50, 200, 80, 220})
		}
	}

	g.drawToast(screen, sw, sh)
	g.drawControlsHelp(screen, sw, sh)
}

func (g *Game) drawToast(screen *ebiten.Image, sw, sh int) {
	if g.toast == "" || !time.Now().Before(g.toastUntil) {
		return
	}

	tx := float64(sw) / 2
	ty := float64(sh)/2 - 40

	alpha := uint8(255)
	if remaining := time.Until(g.toastUntil); remaining < 500*time.Millisecond {
		alpha = uint8(float64(remaining) / float64(500*time.Millisecond) * 255)
	}

	tw := float64(len(g.toast)) * 7
	th := 20.0
	drawRect(screen,
		tx-tw/2-10, ty-th/2-4,
		tw+20, th+8,
		color.RGBA{0, 0, 0, alpha / 2},
	)
	ebitenutil.DebugPrintAt(screen, g.toast, int(tx-tw/2), int(ty-th/2))
}

func (g *Game) drawControlsHelp(screen *ebiten.Image, sw, sh int) {
	controls := strings.Join([]string{
		"Space: Play/Pause",
		"←/→: Seek ±5s (Shift: ±30s)",
		"↑/↓: Volume ±5%",
		"M: Mute",
		"L: Loop",
		"R: Restart",
		"1-9: Speed",
		"Drop: Load file",
		"Q/Esc: Quit",
	}, "  |  ")

	helpX := (sw - len(controls)*6) / 2
	if helpX < 4 {
		helpX = 4
	}
	ebitenutil.DebugPrintAt(screen, controls, helpX, sh-14)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

// runGame configures and starts the Ebiten game loop.
func runGame(game *Game) error {
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(60)
	return ebiten.RunGame(game)
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
