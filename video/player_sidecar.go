//go:build !js && sidecar

package video

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/realskyquest/ebiten-gstreamer/internal/client"
)

// playerSeq gives each sidecar player a unique shared-memory name within the
// process, allowing multiple simultaneous Players from the same Context.
var playerSeq atomic.Uint64

// Player plays a single video by delegating to an out-of-process sidecar over
// TCP + shared memory. The GStreamer runtime lives entirely inside the sidecar
// binary; this process stays CGo-free.
//
// All methods are safe for concurrent use and mirror the native player.go API.
type Player struct {
	ctx  *Context
	opts *PlayerOptions

	inner *client.Player

	// Frame state
	mu       sync.Mutex
	frameImg *ebiten.Image
	imgW     int
	imgH     int
	videoW   int
	videoH   int
}

func newPlayer(ctx *Context, source string, opts *PlayerOptions) (*Player, error) {
	uri := toURI(source)

	p := &Player{
		ctx:  ctx,
		opts: opts,
	}

	cfg := client.DefaultConfig()
	cfg.ShmName = fmt.Sprintf("vp_%d_%d", os.Getpid(), playerSeq.Add(1))

	inner := client.New(cfg, client.EventHandler{
		OnMediaInfo: func(w, h int) {
			p.mu.Lock()
			p.videoW = w
			p.videoH = h
			p.mu.Unlock()
		},
		OnStateChanged: func(state string, playing, eos bool) {},
		OnPosition:     func(pos, dur time.Duration, vol, rate float64) {},
		OnEOS: func() {
			if opts.OnEnd != nil {
				opts.OnEnd()
			}
		},
		OnError: func(message, debug string) {
			if opts.OnError != nil {
				err := &PipelineError{Message: message, Debug: debug}
				opts.OnError(err)
			}
		},
		OnBuffering: func(percent int) {
			if opts.OnBuffering != nil {
				opts.OnBuffering(percent)
			}
		},
	})

	if err := inner.Start(); err != nil {
		return nil, fmt.Errorf("video: start sidecar: %w", err)
	}

	var copts client.PlayerOptions
	copts.Volume = opts.Volume
	copts.Muted = opts.Muted
	copts.Loop = opts.Loop
	copts.Rate = opts.Rate
	if opts.Width > 0 {
		copts.Width = opts.Width
	}
	if opts.Height > 0 {
		copts.Height = opts.Height
	}
	if opts.MaxBufferedFrames > 0 {
		copts.MaxBufferFrames = uint(opts.MaxBufferedFrames)
	}

	if err := inner.Open(uri, copts); err != nil {
		inner.Close()
		return nil, fmt.Errorf("video: open uri: %w", err)
	}

	p.inner = inner
	return p, nil
}

// Frame returns the latest decoded video frame as an *ebiten.Image.
// Call this inside [ebiten.Game.Draw].
// Returns nil if no frame is available yet.
// The returned image is owned by the Player; do not dispose it.
func (p *Player) Frame() *ebiten.Image {
	frame := p.inner.ReadFrame()
	if frame == nil {
		p.mu.Lock()
		img := p.frameImg
		p.mu.Unlock()
		return img
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	w, h := int(frame.Width), int(frame.Height)
	if p.frameImg == nil || p.imgW != w || p.imgH != h {
		if p.frameImg != nil {
			p.frameImg.Deallocate()
		}
		p.frameImg = ebiten.NewImage(w, h)
		p.imgW = w
		p.imgH = h
	}

	p.frameImg.WritePixels(frame.Data)
	return p.frameImg
}

// Play starts or resumes playback.
func (p *Player) Play() {
	if err := p.inner.Play(); err != nil {
		if p.opts.OnError != nil {
			p.opts.OnError(fmt.Errorf("video: Play: %w", err))
		}
		return
	}
}

// Pause pauses playback. Call Play to resume.
func (p *Player) Pause() {
	if err := p.inner.Pause(); err != nil {
		if p.opts.OnError != nil {
			p.opts.OnError(fmt.Errorf("video: Pause: %w", err))
		}
	}
}

// IsPlaying reports whether playback is currently active.
func (p *Player) IsPlaying() bool {
	return p.inner.IsPlaying()
}

// IsEOS reports whether the player has reached end-of-stream.
func (p *Player) IsEOS() bool {
	return p.inner.IsEOS()
}

// Position returns the current playback position.
func (p *Player) Position() time.Duration {
	return p.inner.Position()
}

// Duration returns the total duration of the video.
// Duration returns 0 for live streams or before the duration is known.
func (p *Player) Duration() time.Duration {
	return p.inner.Duration()
}

// SetPosition seeks to the given offset.
func (p *Player) SetPosition(offset time.Duration) error {
	// Seeking after EOS is valid and resets the EOS state inside the sidecar.
	return p.inner.SetPosition(offset)
}

// Rewind seeks to the beginning.
func (p *Player) Rewind() error {
	return p.inner.Rewind()
}

// SetVolume sets the audio volume. Range: 0.0 (muted) to 1.0+ (>1 amplifies).
func (p *Player) SetVolume(v float64) {
	if err := p.inner.SetVolume(v); err != nil {
		if p.opts.OnError != nil {
			p.opts.OnError(fmt.Errorf("video: SetVolume: %w", err))
		}
	}
}

// Volume returns the current audio volume.
func (p *Player) Volume() float64 {
	return p.inner.Volume()
}

// Loop reports whether the video is set to loop.
func (p *Player) Loop() bool {
	return p.inner.Loop()
}

// SetLoop sets whether the video should loop.
func (p *Player) SetLoop(loop bool) {
	if err := p.inner.SetLoop(loop); err != nil {
		if p.opts.OnError != nil {
			p.opts.OnError(fmt.Errorf("video: SetLoop: %w", err))
		}
	}
}

// Rate returns the current playback rate (1.0 = normal speed).
func (p *Player) Rate() float64 {
	return p.inner.Rate()
}

// SetRate sets the playback rate.
func (p *Player) SetRate(rate float64) {
	if err := p.inner.SetRate(rate); err != nil {
		if p.opts.OnError != nil {
			p.opts.OnError(fmt.Errorf("video: SetRate: %w", err))
		}
	}
}

// VideoSize returns the video dimensions (width, height).
// VideoSize returns (0, 0) before the first frame arrives.
func (p *Player) VideoSize() (int, int) {
	return p.inner.VideoSize()
}

// Close releases all resources, including the sidecar process.
func (p *Player) Close() {
	p.mu.Lock()
	if p.frameImg != nil {
		p.frameImg.Deallocate()
		p.frameImg = nil
	}
	p.mu.Unlock()

	p.inner.Close()
	if p.ctx != nil {
		p.ctx.removePlayer(p)
	}
}
