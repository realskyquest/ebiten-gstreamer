package client

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/realskyquest/ebiten-gstreamer/internal/protocol"
	"github.com/realskyquest/ebiten-gstreamer/internal/shm"
)

// Frame holds a decoded RGBA frame copied from shared memory.
type Frame struct {
	Width  uint32
	Height uint32
	Stride uint32
	PtsNs  int64
	Number uint64
	Data   []byte // RGBA pixels, len = Stride*Height
}

// EventHandler receives asynchronous events from the sidecar.
type EventHandler struct {
	OnStateChanged func(state string, playing, eos bool)
	OnPosition     func(position, duration time.Duration, volume, rate float64)
	OnMediaInfo    func(width, height int)
	OnError        func(message, debug string)
	OnEOS          func()
	OnBuffering    func(percent int)
}

type PlayerOptions struct {
	Width           int
	Height          int
	MaxBufferFrames uint
	Volume          float64
	Loop            bool
	Rate            float64
}

func DefaultPlayerOptions() PlayerOptions {
	return PlayerOptions{
		Volume: 1.0,
		Rate:   1.0,
	}
}

// Config for the sidecar process.
type Config struct {
	SidecarBin string
	ShmName    string
	MaxWidth   uint32
	MaxHeight  uint32
}

// sidecarBin returns the platform-appropriate sidecar binary name.
// On Windows it looks for ./sidecar.exe; on all other platforms ./sidecar.
func sidecarBin() string {
	if runtime.GOOS == "windows" {
		return ".\\sidecar.exe"
	}
	return "./sidecar"
}

func DefaultConfig() Config {
	return Config{
		SidecarBin: sidecarBin(),
		ShmName:    fmt.Sprintf("vp_%d", os.Getpid()),
		MaxWidth:   3840,
		MaxHeight:  2160,
	}
}

// Player manages the sidecar lifecycle, IPC, and frame reading.
// Safe for concurrent use.
type Player struct {
	cfg     Config
	handler EventHandler

	mu       sync.RWMutex
	conn     *protocol.Conn
	mem      *shm.SharedMem
	cmd      *exec.Cmd
	playing  bool
	eos      bool
	loop     bool
	position int64
	duration int64
	volume   float64
	rate     float64
	videoW   int
	videoH   int

	lastFrame uint64
	frameBuf  []byte

	ctx    context.Context
	cancel context.CancelFunc
}

func New(cfg Config, handler EventHandler) *Player {
	ctx, cancel := context.WithCancel(context.Background())
	maxBytes := int(cfg.MaxWidth) * int(cfg.MaxHeight) * 4
	return &Player{
		cfg:      cfg,
		handler:  handler,
		volume:   1.0,
		rate:     1.0,
		frameBuf: make([]byte, maxBytes),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start launches the sidecar process, connects TCP, and opens shared memory.
func (p *Player) Start() error {
	cmd := exec.CommandContext(p.ctx, p.cfg.SidecarBin,
		"--port", "0",
		"--shm", p.cfg.ShmName,
		"--max-width", fmt.Sprintf("%d", p.cfg.MaxWidth),
		"--max-height", fmt.Sprintf("%d", p.cfg.MaxHeight),
	)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start sidecar: %w", err)
	}
	p.cmd = cmd

	port, err := readPort(stdout)
	if err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("read port: %w", err)
	}
	go io.Copy(io.Discard, stdout)

	raw, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second)
	if err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("connect: %w", err)
	}
	p.conn = protocol.NewConn(raw)

	msg, err := p.conn.Receive()
	if err != nil || msg.Type != protocol.EvtReady {
		p.Close()
		return fmt.Errorf("expected EvtReady, got err=%v", err)
	}

	p.mem, err = shm.Open(p.cfg.ShmName, p.cfg.MaxWidth, p.cfg.MaxHeight)
	if err != nil {
		p.Close()
		return fmt.Errorf("shm open: %w", err)
	}

	go p.eventLoop()
	go func() { cmd.Wait(); p.cancel() }()

	return nil
}

// Open opens a media URI. This builds the pipeline in the sidecar.
func (p *Player) Open(uri string, opts PlayerOptions) error {
	return p.conn.Send(protocol.CmdOpen, &protocol.OpenPayload{
		URI:             uri,
		Width:           opts.Width,
		Height:          opts.Height,
		MaxBufferFrames: opts.MaxBufferFrames,
		Volume:          opts.Volume,
		Loop:            opts.Loop,
		Rate:            opts.Rate,
	})
}

func (p *Player) Play() error {
	return p.conn.Send(protocol.CmdPlay, nil)
}

func (p *Player) Pause() error {
	return p.conn.Send(protocol.CmdPause, nil)
}

func (p *Player) Stop() error {
	return p.conn.Send(protocol.CmdStop, nil)
}

// SetPosition seeks to the given offset.
func (p *Player) SetPosition(offset time.Duration) error {
	return p.conn.Send(protocol.CmdSeek, &protocol.SeekPayload{
		PositionNs: offset.Nanoseconds(),
	})
}

// Rewind seeks to the start.
func (p *Player) Rewind() error {
	return p.conn.Send(protocol.CmdRewind, nil)
}

func (p *Player) SetVolume(v float64) error {
	return p.conn.Send(protocol.CmdSetVolume, &protocol.VolumePayload{Volume: v})
}

func (p *Player) SetRate(rate float64) error {
	return p.conn.Send(protocol.CmdSetRate, &protocol.RatePayload{Rate: rate})
}

func (p *Player) SetLoop(loop bool) error {
	return p.conn.Send(protocol.CmdSetLoop, &protocol.LoopPayload{Loop: loop})
}

func (p *Player) IsPlaying() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.playing
}

func (p *Player) IsEOS() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.eos
}

func (p *Player) Position() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return time.Duration(p.position)
}

func (p *Player) Duration() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return time.Duration(p.duration)
}

func (p *Player) Volume() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.volume
}

func (p *Player) Rate() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.rate
}

func (p *Player) Loop() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.loop
}

func (p *Player) VideoSize() (int, int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.videoW, p.videoH
}

// ReadFrame returns the latest RGBA frame from shared memory, or nil if
// no new frame is available. Call from your render loop.
// Returned []byte is a fresh copy; safe to hold.
func (p *Player) ReadFrame() *Frame {
	if p.mem == nil {
		return nil
	}

	hdr, isNew := p.mem.ReadFrame(p.frameBuf, p.lastFrame)
	if !isNew {
		return nil
	}
	p.lastFrame = hdr.FrameNumber

	n := int(hdr.Stride * hdr.Height)
	pixels := make([]byte, n)
	copy(pixels, p.frameBuf[:n])

	return &Frame{
		Width:  hdr.Width,
		Height: hdr.Height,
		Stride: hdr.Stride,
		PtsNs:  hdr.PtsNs,
		Number: hdr.FrameNumber,
		Data:   pixels,
	}
}

func (p *Player) Close() error {
	if p.conn != nil {
		p.conn.Send(protocol.CmdShutdown, nil)
		// Give sidecar a moment to clean up
		time.Sleep(100 * time.Millisecond)
	}
	p.cancel()
	if p.mem != nil {
		p.mem.Close()
	}
	if p.conn != nil {
		p.conn.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait()
	}
	return nil
}

func (p *Player) Done() <-chan struct{} {
	return p.ctx.Done()
}

func (p *Player) eventLoop() {
	for {
		msg, err := p.conn.Receive()
		if err != nil {
			select {
			case <-p.ctx.Done():
			default:
				log.Printf("client: event read error: %v", err)
			}
			return
		}

		switch msg.Type {
		case protocol.EvtStateChanged:
			pl, _ := protocol.Decode[protocol.StateChangedPayload](msg)
			p.mu.Lock()
			p.playing = pl.Playing
			p.eos = pl.EOS
			p.mu.Unlock()
			if p.handler.OnStateChanged != nil {
				p.handler.OnStateChanged(pl.State, pl.Playing, pl.EOS)
			}

		case protocol.EvtPosition:
			pl, _ := protocol.Decode[protocol.PositionPayload](msg)
			p.mu.Lock()
			p.position = pl.PositionNs
			p.duration = pl.DurationNs
			p.volume = pl.Volume
			p.rate = pl.Rate
			p.playing = pl.Playing
			p.eos = pl.EOS
			p.loop = pl.Loop
			p.mu.Unlock()
			if p.handler.OnPosition != nil {
				p.handler.OnPosition(
					time.Duration(pl.PositionNs),
					time.Duration(pl.DurationNs),
					pl.Volume,
					pl.Rate,
				)
			}

		case protocol.EvtMediaInfo:
			pl, _ := protocol.Decode[protocol.MediaInfoPayload](msg)
			p.mu.Lock()
			p.videoW = pl.Width
			p.videoH = pl.Height
			p.mu.Unlock()
			if p.handler.OnMediaInfo != nil {
				p.handler.OnMediaInfo(pl.Width, pl.Height)
			}

		case protocol.EvtError:
			pl, _ := protocol.Decode[protocol.ErrorPayload](msg)
			if p.handler.OnError != nil {
				p.handler.OnError(pl.Message, pl.Debug)
			}

		case protocol.EvtEOS:
			p.mu.Lock()
			p.eos = true
			p.playing = false
			p.mu.Unlock()
			if p.handler.OnEOS != nil {
				p.handler.OnEOS()
			}

		case protocol.EvtBuffering:
			pl, _ := protocol.Decode[protocol.BufferingPayload](msg)
			if p.handler.OnBuffering != nil {
				p.handler.OnBuffering(pl.Percent)
			}
		}
	}
}

func readPort(r io.Reader) (int, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PORT:") {
			var port int
			if _, err := fmt.Sscanf(line, "PORT:%d", &port); err == nil && port > 0 {
				return port, nil
			}
		}
	}
	return 0, fmt.Errorf("sidecar did not report port")
}
