//go:build js && wasm

package video

import (
	"fmt"
	"sync/atomic"
	"syscall/js"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// initContext is a no-op on JS/Wasm; no native runtime needs initializing.
func initContext(_ *Context) error {
	return nil
}

// Player implements video playback via an HTML5 <video> element.
// All methods are safe for concurrent use.
type Player struct {
	ctx    *Context
	opts   *PlayerOptions
	volume float64

	video  js.Value // <video> DOM element
	canvas js.Value // offscreen <canvas> for pixel readback
	jsCtx  js.Value // 2D rendering context of canvas

	videoImage *ebiten.Image
	pixels     []byte

	videoW, videoH int

	// jsFuncs holds all js.Func values so they can be released on Close.
	jsFuncs []js.Func

	ready   atomic.Bool
	closed  atomic.Bool
	playing atomic.Bool
	eos     atomic.Bool
}

// newPlayer creates a Player backed by an HTML5 <video> element.
// It blocks until the video metadata is loaded (videoWidth/videoHeight are known),
// or returns an error after a 10-second timeout.
func newPlayer(ctx *Context, src string, opts *PlayerOptions) (*Player, error) {
	p := &Player{
		ctx:    ctx,
		opts:   opts,
		volume: opts.Volume,
	}

	doc := js.Global().Get("document")

	video := doc.Call("createElement", "video")
	video.Set("src", src)
	video.Set("crossOrigin", "anonymous")
	video.Set("loop", opts.Loop)
	video.Set("muted", opts.Volume == 0)
	video.Set("playsInline", true)
	video.Set("playbackRate", opts.Rate)
	video.Get("style").Set("display", "none")
	doc.Get("body").Call("appendChild", video)

	// Offscreen canvas is used to read pixel data from the video element.
	canvas := doc.Call("createElement", "canvas")
	jsCtx := canvas.Call("getContext", "2d", map[string]interface{}{
		"willReadFrequently": true,
	})

	p.video = video
	p.canvas = canvas
	p.jsCtx = jsCtx

	p.setupEventListeners()

	// Wait for HAVE_METADATA (readyState >= 1) so dimensions are known.
	ready := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for {
			if time.Now().After(deadline) {
				ready <- fmt.Errorf("video: timed out waiting for metadata: %s", src)
				return
			}
			if p.video.Get("readyState").Int() >= 1 {
				p.videoW = video.Get("videoWidth").Int()
				p.videoH = video.Get("videoHeight").Int()

				canvas.Set("width", p.videoW)
				canvas.Set("height", p.videoH)

				p.pixels = make([]byte, p.videoW*p.videoH*4)
				p.videoImage = ebiten.NewImage(p.videoW, p.videoH)

				p.ready.Store(true)
				ready <- nil
				return
			}
			time.Sleep(16 * time.Millisecond)
		}
	}()

	if err := <-ready; err != nil {
		p.releaseJSFuncs()
		video.Get("parentNode").Call("removeChild", video)
		return nil, err
	}

	return p, nil
}

// setupEventListeners attaches DOM event handlers to the <video> element.
// All js.Func values are stored in p.jsFuncs so they can be released on Close.
func (p *Player) setupEventListeners() {
	add := func(event string, fn func(js.Value, []js.Value) interface{}) {
		f := js.FuncOf(fn)
		p.jsFuncs = append(p.jsFuncs, f)
		p.video.Call("addEventListener", event, f)
	}

	add("ended", func(_ js.Value, _ []js.Value) interface{} {
		p.eos.Store(true)
		p.playing.Store(false)
		if p.opts.Loop {
			p.video.Set("currentTime", 0)
			p.video.Call("play")
		} else if p.opts.OnEnd != nil {
			p.opts.OnEnd()
		}
		return nil
	})

	add("error", func(_ js.Value, _ []js.Value) interface{} {
		if p.opts.OnError != nil {
			p.opts.OnError(&PipelineError{})
		}
		return nil
	})

	// HTML5 video does not provide a buffering percentage; fire callback with -1
	// to signal that buffering is in progress without a known percentage.
	add("waiting", func(_ js.Value, _ []js.Value) interface{} {
		if p.opts.OnBuffering != nil {
			p.opts.OnBuffering(-1)
		}
		return nil
	})

	add("play", func(_ js.Value, _ []js.Value) interface{} {
		p.playing.Store(true)
		return nil
	})

	add("pause", func(_ js.Value, _ []js.Value) interface{} {
		if !p.video.Get("ended").Bool() {
			p.playing.Store(false)
		}
		return nil
	})
}

// releaseJSFuncs releases all retained js.Func values to allow GC.
func (p *Player) releaseJSFuncs() {
	for _, f := range p.jsFuncs {
		f.Release()
	}
	p.jsFuncs = nil
}

// Frame draws the current video frame onto an *ebiten.Image and returns it.
// Returns nil if the player is not yet ready or has been closed.
// The returned image is owned by the Player and must not be disposed of by the caller.
func (p *Player) Frame() *ebiten.Image {
	if p.closed.Load() || !p.ready.Load() || p.videoImage == nil {
		return nil
	}

	// Return the last known frame without re-reading pixels when not advancing.
	if p.video.Get("paused").Bool() || p.video.Get("ended").Bool() {
		return p.videoImage
	}

	p.jsCtx.Call("drawImage", p.video, 0, 0, p.videoW, p.videoH)
	imgData := p.jsCtx.Call("getImageData", 0, 0, p.videoW, p.videoH)
	js.CopyBytesToGo(p.pixels, imgData.Get("data"))
	p.videoImage.WritePixels(p.pixels)

	return p.videoImage
}

// Play starts or resumes playback.
func (p *Player) Play() {
	if p.closed.Load() {
		panic("video: Play called on closed Player")
	}
	if p.playing.Load() {
		return
	}
	if p.eos.Load() {
		// Restart from beginning regardless of loop setting, matching native behaviour.
		p.video.Set("currentTime", 0)
		p.eos.Store(false)
	}
	p.video.Call("play")
	p.playing.Store(true)
}

// Pause pauses playback. Call Play to resume.
func (p *Player) Pause() {
	if p.closed.Load() {
		panic("video: Pause called on closed Player")
	}
	p.video.Call("pause")
	p.playing.Store(false)
}

// IsPlaying reports whether the video is currently playing.
func (p *Player) IsPlaying() bool {
	return p.playing.Load() && !p.video.Get("paused").Bool() && !p.video.Get("ended").Bool()
}

// IsEOS reports whether the player has reached end-of-stream.
func (p *Player) IsEOS() bool {
	return p.eos.Load()
}

// Position returns the current playback position.
func (p *Player) Position() time.Duration {
	secs := p.video.Get("currentTime").Float()
	return time.Duration(secs * float64(time.Second))
}

// SetPosition seeks to the given offset.
// Returns an error if the offset is out of range or the player is closed.
func (p *Player) SetPosition(offset time.Duration) error {
	if p.closed.Load() {
		return fmt.Errorf("video: SetPosition called on closed Player (%w)", ErrPlayerClosed)
	}
	secs := offset.Seconds()
	dur := p.Duration().Seconds()
	if secs < 0 || secs > dur {
		return fmt.Errorf("video: position %v out of range [0, %v]", offset, p.Duration())
	}
	p.video.Set("currentTime", secs)
	p.eos.Store(false)
	return nil
}

// Rewind seeks back to the beginning.
func (p *Player) Rewind() error {
	return p.SetPosition(0)
}

// Duration returns the total duration of the video.
// Returns 0 if the duration is not yet known.
func (p *Player) Duration() time.Duration {
	secs := p.video.Get("duration").Float()
	if secs != secs { // NaN indicates unknown duration (e.g. live stream)
		return 0
	}
	return time.Duration(secs * float64(time.Second))
}

// SetVolume sets the audio volume. Range: 0.0 (muted) to 1.0 (full).
// Values outside this range are clamped.
func (p *Player) SetVolume(v float64) {
	if p.closed.Load() {
		panic("video: SetVolume called on closed Player")
	}
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	p.video.Set("volume", v)
	p.video.Set("muted", v == 0)
	p.volume = v
}

// Volume returns the current audio volume.
func (p *Player) Volume() float64 {
	return p.volume
}

// Loop returns whether the video is set to loop.
func (p *Player) Loop() bool {
	return p.opts.Loop
}

// SetLoop sets whether the video should loop.
func (p *Player) SetLoop(loop bool) {
	p.video.Set("loop", loop)
	p.opts.Loop = loop
}

// Rate returns the current playback rate (1.0 = normal speed).
func (p *Player) Rate() float64 {
	return p.opts.Rate
}

// SetRate sets the playback rate. 1.0 is normal, 0.5 is half, 2.0 is double.
func (p *Player) SetRate(rate float64) {
	if p.closed.Load() {
		panic("video: SetRate called on closed Player")
	}
	p.video.Set("playbackRate", rate)
	p.opts.Rate = rate
}

// VideoSize returns the natural width and height of the video in pixels.
// Returns (0, 0) before metadata has loaded.
func (p *Player) VideoSize() (int, int) {
	return p.videoW, p.videoH
}

// Close removes the <video> element from the DOM and releases all resources.
func (p *Player) Close() {
	if p.closed.CompareAndSwap(false, true) {
		p.video.Call("pause")
		p.video.Get("parentNode").Call("removeChild", p.video)
		p.releaseJSFuncs()
		p.videoImage = nil
		p.pixels = nil
		p.ready.Store(false)

		if p.ctx != nil {
			p.ctx.removePlayer(p)
		}
	}
}
