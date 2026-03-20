//go:build !js

// Package main implements the ebiten-gstreamer sidecar process.
//
// The sidecar is a helper binary that isolates GStreamer and CGo from the
// host application. One sidecar process is launched per video.Player when
// built with the "sidecar" build tag.
//
// Each sidecar instance serves exactly one TCP client connection. After the
// client disconnects (or sends CmdShutdown), the process exits. This is by
// design: shared memory is sized at startup and cannot be reconfigured while
// a client is connected. To play multiple videos simultaneously, the host
// application launches one sidecar per player (handled automatically by
// internal/client).

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"

	"github.com/realskyquest/ebiten-gstreamer/internal/protocol"
	"github.com/realskyquest/ebiten-gstreamer/internal/shm"
	"github.com/realskyquest/ebiten-gstreamer/videosidecar"
)

func main() {
	port := flag.Int("port", 0, "TCP listen port (0 = random)")
	shmName := flag.String("shm", "default", "Shared memory name")
	maxW := flag.Uint("max-width", 3840, "Max frame width")
	maxH := flag.Uint("max-height", 2160, "Max frame height")
	flag.Parse()

	log.SetPrefix("[sidecar] ")
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)

	gst.Init(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Shared memory
	mem, err := shm.Create(*shmName, uint32(*maxW), uint32(*maxH))
	if err != nil {
		log.Fatalf("%s: %v", videosidecar.ErrSidecarShmCreate, err)
	}
	defer mem.Close()

	// TCP listener
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatalf("%s: %v", videosidecar.ErrSidecarListen, err)
	}
	defer ln.Close()

	fmt.Fprintf(os.Stdout, "PORT:%d\n", ln.Addr().(*net.TCPAddr).Port)
	os.Stdout.Sync()

	raw, err := ln.Accept()
	if err != nil {
		log.Fatalf("%s: %v", videosidecar.ErrSidecarAccept, err)
	}
	// Stop listening, this process serves exactly one client connection.
	// The host launches a separate sidecar process per video.Player.
	ln.Close()
	conn := protocol.NewConn(raw)
	defer conn.Close()

	s := &sidecar{
		mem:    mem,
		conn:   conn,
		maxW:   uint32(*maxW),
		maxH:   uint32(*maxH),
		ctx:    ctx,
		cancel: cancel,
	}

	conn.Send(protocol.EvtReady, nil)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
		case <-ctx.Done():
		}
		cancel()
	}()

	// Position ticker
	go s.positionLoop()

	// Command loop (blocks)
	s.commandLoop()
}

type sidecar struct {
	mem  *shm.SharedMem
	conn *protocol.Conn
	maxW uint32
	maxH uint32

	mu       sync.Mutex
	pipeline *gst.Pipeline
	volume   *gst.Element // volume element
	sink     *app.Sink
	mainLoop *glib.MainLoop

	videoW int
	videoH int

	playing  atomic.Bool
	closed   atomic.Bool
	eos      atomic.Bool
	frameNum uint64
	loop     bool
	rate     float64
	vol      float64

	loopDone chan struct{} // glib main loop done
	pullDone chan struct{} // sample puller done

	sizeOnce sync.Once

	ctx    context.Context
	cancel context.CancelFunc
}

func (s *sidecar) commandLoop() {
	for {
		msg, err := s.conn.Receive()
		if err != nil {
			log.Printf("client disconnected: %v", err)
			s.teardown()
			s.cancel()
			return
		}

		switch msg.Type {
		case protocol.CmdOpen:
			var pl protocol.OpenPayload
			if s.decode(msg, &pl) {
				s.cmdOpen(pl)
			}

		case protocol.CmdPlay:
			s.cmdPlay()

		case protocol.CmdPause:
			s.cmdPause()

		case protocol.CmdStop:
			s.cmdStop()

		case protocol.CmdSeek:
			var pl protocol.SeekPayload
			if s.decode(msg, &pl) {
				s.cmdSeek(pl.PositionNs)
			}

		case protocol.CmdSetVolume:
			var pl protocol.VolumePayload
			if s.decode(msg, &pl) {
				s.cmdSetVolume(pl.Volume)
			}

		case protocol.CmdSetRate:
			var pl protocol.RatePayload
			if s.decode(msg, &pl) {
				s.cmdSetRate(pl.Rate)
			}

		case protocol.CmdSetLoop:
			var pl protocol.LoopPayload
			if s.decode(msg, &pl) {
				s.mu.Lock()
				s.loop = pl.Loop
				s.mu.Unlock()
			}

		case protocol.CmdRewind:
			s.cmdSeek(0)

		case protocol.CmdShutdown:
			s.teardown()
			s.conn.Send(protocol.EvtShutdownAck, nil)
			s.cancel()
			return
		}
	}
}

func (s *sidecar) cmdOpen(opts protocol.OpenPayload) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Tear down any previous pipeline
	s.teardownLocked()

	// Reset state
	s.sizeOnce = sync.Once{}
	s.videoW = 0
	s.videoH = 0
	s.playing.Store(false)
	s.eos.Store(false)
	s.closed.Store(false)
	atomic.StoreUint64(&s.frameNum, 0)

	s.loop = opts.Loop
	s.rate = opts.Rate
	if s.rate == 0 {
		s.rate = 1.0
	}
	s.vol = opts.Volume
	if s.vol == 0 {
		s.vol = 1.0
	}

	// Create pipeline
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		s.sendError(videosidecar.ErrSidecarPipeline, videosidecar.MsgCreatePipeline, err)
		return
	}
	s.pipeline = pipeline

	// uridecodebin
	src, err := gst.NewElement("uridecodebin")
	if err != nil {
		s.sendError(videosidecar.ErrSidecarUridecodebin, videosidecar.MsgUridecodebin, err)
		return
	}
	if err := src.SetProperty("uri", opts.URI); err != nil {
		s.sendError(videosidecar.ErrSidecarSetURI, videosidecar.MsgSetURI, err)
		return
	}

	// Video branch
	vconv, err := gst.NewElement("videoconvert")
	if err != nil {
		s.sendError(videosidecar.ErrSidecarVideoconvert, videosidecar.MsgVideoconvert, err)
		return
	}
	vscale, err := gst.NewElement("videoscale")
	if err != nil {
		s.sendError(videosidecar.ErrSidecarVideoscale, videosidecar.MsgVideoscale, err)
		return
	}
	capsFilter, err := gst.NewElement("capsfilter")
	if err != nil {
		s.sendError(videosidecar.ErrSidecarCapsfilter, videosidecar.MsgCapsfilter, err)
		return
	}

	capsStr := "video/x-raw,format=RGBA"
	if opts.Width > 0 && opts.Height > 0 {
		capsStr += fmt.Sprintf(",width=%d,height=%d", opts.Width, opts.Height)
	}
	caps := gst.NewCapsFromString(capsStr)
	if err := capsFilter.SetProperty("caps", caps); err != nil {
		s.sendError(videosidecar.ErrSidecarSetCaps, videosidecar.MsgSetCaps, err)
		return
	}

	videoSink, err := app.NewAppSink()
	if err != nil {
		s.sendError(videosidecar.ErrSidecarAppsink, videosidecar.MsgAppsink, err)
		return
	}
	videoSink.SetDrop(true)
	maxBuf := opts.MaxBufferFrames
	if maxBuf == 0 {
		maxBuf = 2
	}
	videoSink.SetMaxBuffers(maxBuf)
	s.sink = videoSink

	// Audio branch
	aconv, err := gst.NewElement("audioconvert")
	if err != nil {
		s.sendError(videosidecar.ErrSidecarAudioconvert, videosidecar.MsgAudioconvert, err)
		return
	}
	aresample, err := gst.NewElement("audioresample")
	if err != nil {
		s.sendError(videosidecar.ErrSidecarAudioresample, videosidecar.MsgAudioresample, err)
		return
	}
	vol, err := gst.NewElement("volume")
	if err != nil {
		s.sendError(videosidecar.ErrSidecarVolume, videosidecar.MsgVolume, err)
		return
	}
	if err := vol.SetProperty("volume", s.vol); err != nil {
		s.sendError(videosidecar.ErrSidecarSetVolume, videosidecar.MsgSetVolume, err)
		return
	}
	s.volume = vol

	asink, err := gst.NewElement("autoaudiosink")
	if err != nil {
		s.sendError(videosidecar.ErrSidecarAutoaudiosink, videosidecar.MsgAutoaudiosink, err)
		return
	}

	// Assemble pipeline
	pipeline.AddMany(
		src,
		vconv, vscale, capsFilter, videoSink.Element,
		aconv, aresample, vol, asink,
	)

	// Link video branch: vconv → vscale → capsFilter → appsink
	if err := gst.ElementLinkMany(vconv, vscale, capsFilter, videoSink.Element); err != nil {
		s.sendError(videosidecar.ErrSidecarLinkVideo, videosidecar.MsgLinkVideoBranch, err)
		return
	}

	// Link audio branch: aconv → aresample → volume → autoaudiosink
	if err := gst.ElementLinkMany(aconv, aresample, vol, asink); err != nil {
		s.sendError(videosidecar.ErrSidecarLinkAudio, videosidecar.MsgLinkAudioBranch, err)
		return
	}

	// Dynamic pad linking (identical to pad-added handler)
	src.Connect("pad-added", func(self *gst.Element, pad *gst.Pad) {
		caps := pad.GetCurrentCaps()
		if caps == nil {
			return
		}
		st := caps.GetStructureAt(0)
		if st == nil {
			return
		}

		name := st.Name()
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

	// Bus watch
	s.setupBusWatch()

	// Goroutines
	s.loopDone = make(chan struct{})
	s.pullDone = make(chan struct{})

	go s.runMainLoop()
	go s.pullSamples()

	// Start paused (pre-roll decodes first frame, exactly like your player)
	pipeline.SetState(gst.StatePaused)
}

func (s *sidecar) setupBusWatch() {
	bus := s.pipeline.GetPipelineBus()
	bus.AddWatch(func(msg *gst.Message) bool {
		if s.closed.Load() {
			return false
		}

		switch msg.Type() {
		case gst.MessageEOS:
			s.handleEOS()

		case gst.MessageError:
			gerr := msg.ParseError()
			pErr := protocol.ErrorPayload{
				Message: gerr.Error(),
			}
			if debug := gerr.DebugString(); debug != "" {
				pErr.Debug = debug
			}
			s.conn.Send(protocol.EvtError, &pErr)

		case gst.MessageBuffering:
			st := msg.GetStructure()
			if st != nil {
				if val, err := st.GetValue("buffer-percent"); err == nil {
					if pct, ok := val.(int); ok {
						s.conn.Send(protocol.EvtBuffering, &protocol.BufferingPayload{
							Percent: pct,
						})
					}
				}
			}
		}
		return true
	})
}

func (s *sidecar) handleEOS() {
	s.mu.Lock()
	loop := s.loop
	pipeline := s.pipeline
	s.mu.Unlock()

	if loop && pipeline != nil {
		pipeline.SeekTime(0, gst.SeekFlagFlush|gst.SeekFlagKeyUnit)
		return
	}

	s.eos.Store(true)
	s.playing.Store(false)
	s.conn.Send(protocol.EvtEOS, nil)
}

func (s *sidecar) runMainLoop() {
	defer close(s.loopDone)
	s.mu.Lock()
	s.mainLoop = glib.NewMainLoop(glib.MainContextDefault(), false)
	ml := s.mainLoop
	s.mu.Unlock()
	ml.Run()
}

// TryPullSample with 100ms timeout, resolve size from caps on first frame,
// then write RGBA data to shared memory.
func (s *sidecar) pullSamples() {
	defer close(s.pullDone)

	for {
		if s.closed.Load() {
			return
		}

		sample := s.sink.TryPullSample(gst.ClockTime(100 * time.Millisecond))
		if sample == nil {
			continue
		}

		buf := sample.GetBuffer()
		if buf == nil {
			continue
		}

		// Resolve video dimensions from caps on first frame
		s.sizeOnce.Do(func() {
			caps := sample.GetCaps()
			if caps != nil {
				st := caps.GetStructureAt(0)
				if st != nil {
					if w, err := st.GetValue("width"); err == nil {
						if wi, ok := w.(int); ok {
							s.videoW = wi
						}
					}
					if h, err := st.GetValue("height"); err == nil {
						if hi, ok := h.(int); ok {
							s.videoH = hi
						}
					}
				}
			}

			// Send media info to client
			if s.videoW > 0 && s.videoH > 0 {
				s.conn.Send(protocol.EvtMediaInfo, &protocol.MediaInfoPayload{
					Width:  s.videoW,
					Height: s.videoH,
				})
			}
		})

		if s.videoW == 0 || s.videoH == 0 {
			continue
		}

		mapInfo := buf.Map(gst.MapRead)
		if mapInfo == nil {
			continue
		}

		dataSize := s.videoW * s.videoH * 4
		srcBytes := mapInfo.Bytes()
		if len(srcBytes) < dataSize {
			buf.Unmap()
			continue
		}

		width := uint32(s.videoW)
		height := uint32(s.videoH)
		stride := width * 4
		num := atomic.AddUint64(&s.frameNum, 1)

		// PTS: use buffer PTS
		pts := int64(buf.PresentationTimestamp())

		s.mem.WriteFrame(width, height, stride, pts, num, srcBytes[:dataSize])

		buf.Unmap()
	}
}

// positionLoop sends periodic position/state updates.
func (s *sidecar) positionLoop() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			pipeline := s.pipeline
			vol := s.vol
			rate := s.rate
			loop := s.loop
			s.mu.Unlock()

			if pipeline == nil {
				continue
			}

			var pos, dur int64
			if ok, p := pipeline.QueryPosition(gst.FormatTime); ok {
				pos = p
			}
			if ok, d := pipeline.QueryDuration(gst.FormatTime); ok {
				dur = d
			}

			s.conn.Send(protocol.EvtPosition, &protocol.PositionPayload{
				PositionNs: pos,
				DurationNs: dur,
				Volume:     vol,
				Rate:       rate,
				Playing:    s.playing.Load(),
				EOS:        s.eos.Load(),
				Loop:       loop,
			})
		}
	}
}

func (s *sidecar) cmdPlay() {
	s.mu.Lock()
	pipeline := s.pipeline
	s.mu.Unlock()

	if pipeline == nil {
		return
	}

	if s.playing.Load() {
		return
	}

	// If EOS, seek to beginning first
	if s.eos.CompareAndSwap(true, false) {
		pipeline.SeekTime(0, gst.SeekFlagFlush|gst.SeekFlagKeyUnit)
	}

	pipeline.SetState(gst.StatePlaying)
	s.playing.Store(true)

	s.conn.Send(protocol.EvtStateChanged, &protocol.StateChangedPayload{
		State: "playing", Playing: true, EOS: false,
	})
}

func (s *sidecar) cmdPause() {
	s.mu.Lock()
	pipeline := s.pipeline
	s.mu.Unlock()

	if pipeline == nil {
		return
	}

	pipeline.SetState(gst.StatePaused)
	s.playing.Store(false)

	s.conn.Send(protocol.EvtStateChanged, &protocol.StateChangedPayload{
		State: "paused", Playing: false, EOS: s.eos.Load(),
	})
}

func (s *sidecar) cmdStop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teardownLocked()

	s.conn.Send(protocol.EvtStateChanged, &protocol.StateChangedPayload{
		State: "stopped", Playing: false, EOS: false,
	})
}

func (s *sidecar) cmdSeek(posNs int64) {
	s.mu.Lock()
	pipeline := s.pipeline
	s.mu.Unlock()

	if pipeline == nil {
		return
	}

	// SeekTime with Flush|Accurate
	ok := pipeline.SeekTime(time.Duration(posNs), gst.SeekFlagFlush|gst.SeekFlagAccurate)
	if !ok {
		s.sendError(videosidecar.ErrSidecarSeekFailed, videosidecar.MsgSeekFailed, nil)
		return
	}
	s.eos.Store(false)
}

func (s *sidecar) cmdSetVolume(v float64) {
	s.mu.Lock()
	s.vol = v
	volElem := s.volume
	s.mu.Unlock()

	if volElem != nil {
		volElem.SetProperty("volume", v)
	}
}

func (s *sidecar) cmdSetRate(rate float64) {
	s.mu.Lock()
	pipeline := s.pipeline
	s.rate = rate
	s.mu.Unlock()

	if pipeline == nil {
		return
	}

	if rate == 0 {
		s.cmdPause()
		return
	}

	// Get current position
	var pos int64
	if ok, p := pipeline.QueryPosition(gst.FormatTime); ok {
		pos = p
	}

	var ev *gst.Event
	if rate > 0 {
		ev = gst.NewSeekEvent(
			rate,
			gst.FormatTime,
			gst.SeekFlagFlush|gst.SeekFlagKeyUnit,
			gst.SeekTypeSet,
			pos,
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
			pos,
		)
	}

	pipeline.SendEvent(ev)
}

func (s *sidecar) teardown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teardownLocked()
}

func (s *sidecar) teardownLocked() {
	if s.pipeline == nil {
		return
	}

	s.closed.Store(true)
	s.pipeline.BlockSetState(gst.StateNull)
	s.playing.Store(false)

	if s.mainLoop != nil {
		s.mainLoop.Quit()
	}

	// Wait for goroutines
	if s.loopDone != nil {
		<-s.loopDone
	}
	if s.pullDone != nil {
		<-s.pullDone
	}

	s.pipeline = nil
	s.volume = nil
	s.sink = nil
	s.mainLoop = nil
}

func (s *sidecar) decode(msg *protocol.Message, v any) bool {
	if err := json.Unmarshal(msg.Payload, v); err != nil {
		errMsg := fmt.Sprintf("malformed %T payload: %v", v, err)
		log.Printf("ERROR: %s", errMsg)
		s.conn.Send(protocol.EvtError, &protocol.ErrorPayload{Message: errMsg})
		return false
	}
	return true
}

func (s *sidecar) sendError(err error, baseMsg string, underlying error) {
	msg := baseMsg
	if underlying != nil {
		msg = fmt.Sprintf("%s - %s - %v", err, baseMsg, underlying)
	}
	log.Printf("ERROR: %s", msg)
	s.conn.Send(protocol.EvtError, &protocol.ErrorPayload{Message: msg})
}
