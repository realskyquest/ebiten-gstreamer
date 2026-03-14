package main

import (
	"fmt"
	"image/color"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/realskyquest/ebiten-gstreamer/video"
)

const (
	seekStep      = 5 * time.Second
	seekStepLarge = 30 * time.Second
	volumeStep    = 0.05
)

// Video file extensions we accept from drag-and-drop.
var videoExts = map[string]bool{
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

// Cached 1x1 white pixel used by drawRect.
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

	// Track temp files so we can clean them up
	tempFile string
}

func (g *Game) showToast(msg string) {
	g.toast = msg
	g.toastUntil = time.Now().Add(1500 * time.Millisecond)
}

// loadVideo closes the current player (if any) and opens a new video from the given source.
func (g *Game) loadVideo(source string) {
	// Close existing player
	if g.player != nil {
		g.player.Close()
		g.player = nil
	}

	player, err := g.videoCtx.NewPlayer(source, &video.PlayerOptions{
		Volume: g.currentVolume(),
		OnEnd: func() {
			log.Println("Video ended")
		},
		OnError: func(err error) {
			log.Println("Pipeline error:", err)
		},
	})
	if err != nil {
		log.Printf("Failed to load video: %v", err)
		g.showToast(fmt.Sprintf("Error: %s", err))
		return
	}

	g.player = player
	g.player.Play()

	display := source
	if len(display) > 50 {
		display = "..." + display[len(display)-47:]
	}
	g.showToast(fmt.Sprintf("Loaded: %s", display))
	ebiten.SetWindowTitle(fmt.Sprintf("Video Player — %s", filepath.Base(source)))
}

// loadVideoFromFS reads a dropped file from the virtual fs.FS, writes it to a
// temp file (since GStreamer needs a real path), and plays it.
func (g *Game) loadVideoFromFS(droppedFS fs.FS, name string) {
	src, err := droppedFS.Open(name)
	if err != nil {
		log.Printf("Failed to open dropped file: %v", err)
		g.showToast(fmt.Sprintf("Error opening: %s", name))
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

	// Clean up the OLD temp file AFTER the new one is ready,
	// but BEFORE we overwrite g.tempFile
	g.cleanupTemp()

	g.tempFile = tmpFile.Name()
	g.loadVideo(g.tempFile)
}

func (g *Game) cleanupTemp() {
	if g.tempFile != "" {
		os.Remove(g.tempFile)
		g.tempFile = ""
	}
}

func (g *Game) currentVolume() float64 {
	if g.player != nil {
		return g.player.Volume()
	}
	return 0.8
}

func (g *Game) Update() error {
	// Handle dropped files
	if droppedFS := ebiten.DroppedFiles(); droppedFS != nil {
		// Walk the root of the virtual FS to find dropped files
		entries, err := fs.ReadDir(droppedFS, ".")
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				ext := strings.ToLower(filepath.Ext(entry.Name()))
				if videoExts[ext] {
					g.loadVideoFromFS(droppedFS, entry.Name())
					break // load first video file found
				}
			}
		}
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

	// Restart / Rewind to beginning
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

	// Seek
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

	// Volume
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		if g.muted {
			g.muted = false
		}
		vol := clampVol(p.Volume() + volumeStep)
		p.SetVolume(vol)
		g.showToast(fmt.Sprintf("Volume %d%%", int(math.Round(vol*100))))
	}
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
		p.SetLoop(!g.player.Loop())
		if g.player.Loop() {
			g.showToast("Loop ON")
		} else {
			g.showToast("Loop OFF")
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

	p := g.player

	// No video loaded, show drop prompt
	if p == nil {
		msg := "Drag and drop a video file here to play"
		msgW := len(msg) * 6
		ebitenutil.DebugPrintAt(screen, msg, (sw-msgW)/2, sh/2-8)

		// Still draw controls help
		g.drawControlsHelp(screen, sw, sh)
		g.drawToast(screen, sw, sh)
		return
	}

	// Draw video frame
	frame := p.Frame()
	if frame != nil {
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

	// Draw HUD overlay
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
	if g.player.Loop() {
		loopStr = "ON"
	}

	info := fmt.Sprintf(
		"%s  |  %s / %s  |  Vol: %d%%  |  Loop: %s",
		state,
		fmtDur(pos), fmtDur(dur),
		int(math.Round(vol*100)),
		loopStr,
	)
	ebitenutil.DebugPrint(screen, info)

	// Draw progress bar
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

	// Toast and controls help
	g.drawToast(screen, sw, sh)
	g.drawControlsHelp(screen, sw, sh)
}

func (g *Game) drawToast(screen *ebiten.Image, sw, sh int) {
	if time.Now().Before(g.toastUntil) && g.toast != "" {
		tx := float64(sw) / 2
		ty := float64(sh)/2 - 40

		alpha := uint8(255)
		remaining := time.Until(g.toastUntil)
		if remaining < 500*time.Millisecond {
			alpha = uint8(float64(remaining) / float64(500*time.Millisecond) * 255)
		}

		tw := float64(len(g.toast)) * 7
		th := 20.0
		drawRect(screen,
			tx-tw/2-10, ty-th/2-4,
			tw+20, th+8,
			color.RGBA{0, 0, 0, alpha / 2},
		)

		toastX := int(tx - tw/2)
		toastY := int(ty - th/2)
		ebitenutil.DebugPrintAt(screen, g.toast, toastX, toastY)
	}
}

func (g *Game) drawControlsHelp(screen *ebiten.Image, sw, sh int) {
	controls := strings.Join([]string{
		"Space: Play/Pause",
		"←/→: Seek ±5s (Shift: ±30s)",
		"↑/↓: Volume ±5%",
		"M: Mute",
		"L: Loop",
		"R: Restart",
		"Drop: Load file",
		"Q/Esc: Quit",
	}, "  |  ")

	helpY := sh - 14
	helpX := (sw - len(controls)*6) / 2
	if helpX < 4 {
		helpX = 4
	}
	ebitenutil.DebugPrintAt(screen, controls, helpX, helpY)
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

	// If a file/URL was passed as argument, load it immediately
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
		}
	}

	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("Video Player, Drop a file to play")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(60)

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}

	// Cleanup temp file on exit
	game.cleanupTemp()
}
