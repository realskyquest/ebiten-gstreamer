//go:build !js && !sidecar

package video

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
	"github.com/hajimehoshi/ebiten/v2"
)

// initContext performs platform-specific Context initialization.
// On native it initializes the GStreamer runtime.
func initContext(_ *Context) error {
	gst.Init(nil)
	return nil
}

// Player plays a single video. Each Player owns its own GStreamer pipeline.
// All methods are safe for concurrent use.
type Player struct {
	ctx  *Context
	opts *PlayerOptions
	uri  string

	pipeline *gst.Pipeline
	volume   *gst.Element
	sink     *app.Sink
	mainLoop *glib.MainLoop

	// Double-buffered frames. The decode goroutine writes to back, Draw reads front.
	frontBuf []byte
	backBuf  []byte
	frameMu  sync.Mutex
	dirty    atomic.Bool

	// The ebiten.Image exposed to the user.
	image    *ebiten.Image
	imgW     int
	imgH     int
	videoW   int
	videoH   int
	sizeOnce sync.Once

	// State
	playing atomic.Bool
	closed  atomic.Bool
	eos     atomic.Bool

	// Goroutine lifecycle
	loopDone chan struct{}
	pullDone chan struct{}
}

func newPlayer(ctx *Context, source string, opts *PlayerOptions) (*Player, error) {
	uri := toURI(source)

	p := &Player{
		ctx:      ctx,
		opts:     opts,
		uri:      uri,
		loopDone: make(chan struct{}),
		pullDone: make(chan struct{}),
	}

	if err := p.buildPipeline(); err != nil {
		return nil, fmt.Errorf("video: failed to build pipeline: %w", err)
	}

	go p.runMainLoop()
	go p.pullSamples()

	return p, nil
}

func (p *Player) buildPipeline() error {
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		return err
	}
	p.pipeline = pipeline

	// Create elements
	src, err := gst.NewElement("uridecodebin")
	if err != nil {
		return fmt.Errorf("uridecodebin: %w", err)
	}
	if err := src.SetProperty("uri", p.uri); err != nil {
		return fmt.Errorf("set uri: %w", err)
	}

	// Video branch
	vconv, err := gst.NewElement("videoconvert")
	if err != nil {
		return err
	}
	vscale, err := gst.NewElement("videoscale")
	if err != nil {
		return err
	}
	capsFilter, err := gst.NewElement("capsfilter")
	if err != nil {
		return err
	}

	capsStr := "video/x-raw,format=RGBA"
	if p.opts.Width > 0 && p.opts.Height > 0 {
		capsStr += fmt.Sprintf(",width=%d,height=%d", p.opts.Width, p.opts.Height)
	}
	caps := gst.NewCapsFromString(capsStr)
	if err := capsFilter.SetProperty("caps", caps); err != nil {
		return err
	}

	videoSink, err := app.NewAppSink()
	if err != nil {
		return err
	}
	videoSink.SetDrop(true)
	videoSink.SetMaxBuffers(p.opts.MaxBufferedFrames)
	p.sink = videoSink

	// Audio branch
	aconv, err := gst.NewElement("audioconvert")
	if err != nil {
		return err
	}
	aresample, err := gst.NewElement("audioresample")
	if err != nil {
		return err
	}
	vol, err := gst.NewElement("volume")
	if err != nil {
		return err
	}
	if err := vol.SetProperty("volume", p.opts.Volume); err != nil {
		return err
	}
	if p.opts.Muted {
		if err := vol.SetProperty("mute", true); err != nil {
			return err
		}
	}
	p.volume = vol

	asink, err := gst.NewElement("autoaudiosink")
	if err != nil {
		return err
	}

	// Add all to pipeline
	pipeline.AddMany(src, vconv, vscale, capsFilter, videoSink.Element, aconv, aresample, vol, asink)

	// Link video branch: vconv ! vscale ! capsFilter ! videosink
	if err := gst.ElementLinkMany(vconv, vscale, capsFilter, videoSink.Element); err != nil {
		return fmt.Errorf("link video branch: %w", err)
	}

	// Link audio branch: aconv ! aresample ! volume ! autoaudiosink
	if err := gst.ElementLinkMany(aconv, aresample, vol, asink); err != nil {
		return fmt.Errorf("link audio branch: %w", err)
	}

	// Handle dynamic pads from uridecodebin
	src.Connect("pad-added", func(self *gst.Element, pad *gst.Pad) {
		caps := pad.GetCurrentCaps()
		if caps == nil {
			return
		}
		s := caps.GetStructureAt(0)
		if s == nil {
			return
		}

		name := s.Name()
		switch {
		case len(name) >= 5 && name[:5] == "video":
			sinkPad := vconv.GetStaticPad("sink")
			if sinkPad != nil && !sinkPad.IsLinked() {
				pad.Link(sinkPad)
			}
		case len(name) >= 5 && name[:5] == "audio":
			sinkPad := aconv.GetStaticPad("sink")
			if sinkPad != nil && !sinkPad.IsLinked() {
				pad.Link(sinkPad)
			}
		}
	})

	p.setupBusWatch()

	return nil
}

func (p *Player) setupBusWatch() {
	bus := p.pipeline.GetPipelineBus()
	bus.AddWatch(func(msg *gst.Message) bool {
		if p.closed.Load() {
			return false
		}

		switch msg.Type() {
		case gst.MessageEOS:
			p.handleEOS()
		case gst.MessageError:
			gerr := msg.ParseError()
			pErr := &PipelineError{
				Message: gerr.Error(),
			}
			if debug := gerr.DebugString(); debug != "" {
				pErr.Debug = debug
			}
			if p.opts.OnError != nil {
				p.opts.OnError(pErr)
			}
		case gst.MessageBuffering:
			if p.opts.OnBuffering != nil {
				st := msg.GetStructure()
				if st != nil {
					if val, err := st.GetValue("buffer-percent"); err == nil {
						if pct, ok := val.(int); ok {
							p.opts.OnBuffering(pct)
						}
					}
				}
			}
		}
		return true
	})
}

func (p *Player) handleEOS() {
	if p.opts.Loop {
		p.pipeline.SeekTime(0, gst.SeekFlagFlush|gst.SeekFlagKeyUnit)
		return
	}

	p.eos.Store(true)
	p.playing.Store(false)

	if p.opts.OnEnd != nil {
		p.opts.OnEnd()
	}
}

func (p *Player) runMainLoop() {
	defer close(p.loopDone)
	p.mainLoop = glib.NewMainLoop(glib.MainContextDefault(), false)
	p.mainLoop.Run()
}

func (p *Player) pullSamples() {
	defer close(p.pullDone)

	for {
		if p.closed.Load() {
			return
		}

		sample := p.sink.TryPullSample(gst.ClockTime(100 * time.Millisecond))
		if sample == nil {
			continue
		}

		buf := sample.GetBuffer()
		if buf == nil {
			continue
		}

		// Resolve video dimensions from caps on first frame.
		p.sizeOnce.Do(func() {
			caps := sample.GetCaps()
			if caps != nil {
				s := caps.GetStructureAt(0)
				if s != nil {
					if w, err := s.GetValue("width"); err == nil {
						if wi, ok := w.(int); ok {
							p.videoW = wi
						}
					}
					if h, err := s.GetValue("height"); err == nil {
						if hi, ok := h.(int); ok {
							p.videoH = hi
						}
					}
				}
			}
			if p.opts.Width > 0 {
				p.videoW = p.opts.Width
			}
			if p.opts.Height > 0 {
				p.videoH = p.opts.Height
			}
		})

		if p.videoW == 0 || p.videoH == 0 {
			continue
		}

		mapInfo := buf.Map(gst.MapRead)
		if mapInfo == nil {
			continue
		}

		dataSize := p.videoW * p.videoH * 4
		srcBytes := mapInfo.Bytes()
		if len(srcBytes) < dataSize {
			buf.Unmap()
			continue
		}

		p.frameMu.Lock()
		if len(p.backBuf) != dataSize {
			p.backBuf = make([]byte, dataSize)
		}
		copy(p.backBuf, srcBytes[:dataSize])
		p.frameMu.Unlock()
		p.dirty.Store(true)

		buf.Unmap()
	}
}

// Frame returns the current video frame as an *ebiten.Image.
// Call this inside [ebiten.Game.Draw].
//
// Returns nil if no frame is available yet.
// The returned image is owned by the Player and must not be disposed of by the caller.
func (p *Player) Frame() *ebiten.Image {
	if p.closed.Load() {
		return nil
	}

	if p.videoW == 0 || p.videoH == 0 {
		return nil
	}

	if p.dirty.CompareAndSwap(true, false) {
		p.frameMu.Lock()
		p.frontBuf, p.backBuf = p.backBuf, p.frontBuf

		if p.image == nil || p.imgW != p.videoW || p.imgH != p.videoH {
			if p.image != nil {
				p.image.Deallocate()
			}
			p.image = ebiten.NewImage(p.videoW, p.videoH)
			p.imgW = p.videoW
			p.imgH = p.videoH
		}

		p.image.WritePixels(p.frontBuf)
		p.frameMu.Unlock()
	}

	return p.image
}

// Play starts or resumes playback.
func (p *Player) Play() {
	if p.closed.Load() {
		panic("video: Play called on closed Player")
	}
	if p.playing.Load() {
		return
	}

	if p.eos.CompareAndSwap(true, false) {
		p.pipeline.SeekTime(0, gst.SeekFlagFlush|gst.SeekFlagKeyUnit)
	}

	p.pipeline.SetState(gst.StatePlaying)
	p.playing.Store(true)
}

// Pause pauses playback. Call Play to resume.
func (p *Player) Pause() {
	if p.closed.Load() {
		panic("video: Pause called on closed Player")
	}
	p.pipeline.SetState(gst.StatePaused)
	p.playing.Store(false)
}

// IsPlaying returns boolean indicating whether the player is playing.
func (p *Player) IsPlaying() bool {
	return p.playing.Load()
}

// IsEOS reports whether the player has reached end-of-stream.
func (p *Player) IsEOS() bool {
	return p.eos.Load()
}

// SetPosition sets the position with the given offset.
// SetPosition returns error when seeking the source returns error.
func (p *Player) SetPosition(offset time.Duration) error {
	if p.closed.Load() {
		return fmt.Errorf("seek called on (%w)", ErrPlayerClosed)
	}

	ok := p.pipeline.SeekTime(offset, gst.SeekFlagFlush|gst.SeekFlagAccurate)
	if !ok {
		return fmt.Errorf("seek failed (%w)", ErrNotSeekable)
	}

	p.eos.Store(false)
	return nil
}

// Position returns the current position in the time.
func (p *Player) Position() time.Duration {
	if p.closed.Load() {
		return 0
	}
	ok, pos := p.pipeline.QueryPosition(gst.FormatTime)
	if !ok {
		return 0
	}
	return time.Duration(pos)
}

// Rewind rewinds the current position to the start.
// Rewind returns error when seeking the source returns error.
func (p *Player) Rewind() error {
	return p.SetPosition(0)
}

// Duration returns the total duration of the video.
// Duration returns 0 if the duration is unknown (e.g. live stream).
func (p *Player) Duration() time.Duration {
	if p.closed.Load() {
		return 0
	}
	ok, dur := p.pipeline.QueryDuration(gst.FormatTime)
	if !ok {
		return 0
	}
	return time.Duration(dur)
}

// SetVolume sets the audio volume. Range: 0.0 (muted) to 1.0+ (>1 amplifies).
func (p *Player) SetVolume(v float64) {
	if p.closed.Load() {
		panic("video: SetVolume called on closed Player")
	}
	if p.volume != nil {
		p.volume.SetProperty("volume", v)
	}
}

// Volume returns the current audio volume.
func (p *Player) Volume() float64 {
	if p.closed.Load() {
		return 0
	}
	if p.volume == nil {
		return 1.0
	}
	val, err := p.volume.GetProperty("volume")
	if err != nil {
		return 1.0
	}
	if v, ok := val.(float64); ok {
		return v
	}
	return 1.0
}

// Loop returns whether the video should loop.
func (p *Player) Loop() bool {
	return p.opts.Loop
}

// SetLoop sets whether the video should loop.
func (p *Player) SetLoop(loop bool) {
	p.opts.Loop = loop
}

// VideoSize returns the video dimensions (width, height).
// VideoSize returns (0, 0) if not yet known (before first frame).
func (p *Player) VideoSize() (int, int) {
	return p.videoW, p.videoH
}

// Rate returns the current playback rate.
func (p *Player) Rate() float64 {
	return p.opts.Rate
}

// SetRate sets the playback rate.
func (p *Player) SetRate(rate float64) {
	if p.closed.Load() {
		panic("video: SetRate called on closed Player")
	}
	if rate == 0 {
		p.Pause()
		return
	}

	pos := p.Position()

	// Build a seek event with the desired rate
	var ev *gst.Event
	if rate > 0 {
		ev = gst.NewSeekEvent(
			rate,
			gst.FormatTime,
			gst.SeekFlagFlush|gst.SeekFlagKeyUnit,
			gst.SeekTypeSet,
			int64(pos),
			gst.SeekTypeNone,
			0,
		)
	} else {
		ev = gst.NewSeekEvent(
			rate,
			gst.FormatTime,
			gst.SeekFlagFlush|gst.SeekFlagKeyUnit,
			gst.SeekTypeSet,
			0,
			gst.SeekTypeSet,
			int64(pos),
		)
	}

	p.pipeline.SendEvent(ev)
	p.opts.Rate = rate
}

// Close closes the stream.
func (p *Player) Close() {
	if p.closed.CompareAndSwap(false, true) {
		p.pipeline.BlockSetState(gst.StateNull)
		p.playing.Store(false)

		if p.mainLoop != nil {
			p.mainLoop.Quit()
		}

		<-p.loopDone
		<-p.pullDone

		if p.image != nil {
			p.image.Deallocate()
			p.image = nil
		}

		if p.ctx != nil {
			p.ctx.removePlayer(p)
		}
	}
}
