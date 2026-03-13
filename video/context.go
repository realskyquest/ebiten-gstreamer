package video

import (
	"sync"

	"github.com/go-gst/go-gst/gst"
)

var (
	currentContext *Context
	contextMu      sync.Mutex
)

// Context manages GStreamer initialization and provides a factory for Players.
// There should be at most one Context per application, mirroring [audio.Context].
type Context struct {
	mu      sync.Mutex
	closed  bool
	players map[*Player]struct{}
}

// NewContext creates a new video Context and initializes GStreamer.
//
// At most one Context can exist. If a previous Context was created and not closed,
// this panics.
//
// NewContext must be called from the main goroutine or before ebiten.RunGame.
func NewContext() (*Context, error) {
	contextMu.Lock()
	defer contextMu.Unlock()

	if currentContext != nil && !currentContext.closed {
		panic("video: NewContext called while another Context is still open")
	}

	gst.Init(nil)

	ctx := &Context{
		players: make(map[*Player]struct{}),
	}
	currentContext = ctx
	return ctx, nil
}

// CurrentContext returns the current Context, or nil if none exists.
func CurrentContext() *Context {
	contextMu.Lock()
	defer contextMu.Unlock()
	if currentContext != nil && !currentContext.closed {
		return currentContext
	}
	return nil
}

// NewPlayer creates a new Player for the given source.
//
// source can be:
//   - A local file path: "./video.mp4", "/home/user/video.mp4", "C:\\video.mp4"
//   - A file URI: "file:///path/to/video.mp4"
//   - An HTTP(S) URL: "https://example-resources.ebitengine.org/shibuya.mpg"
//   - An RTSP URL: "rtsp://camera.local/stream"
//
// opts may be nil for defaults.
func (c *Context) NewPlayer(source string, opts *PlayerOptions) (*Player, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrContextClosed
	}
	c.mu.Unlock()

	if opts == nil {
		opts = &PlayerOptions{}
	}
	opts.defaults()

	p, err := newPlayer(c, source, opts)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.players[p] = struct{}{}
	c.mu.Unlock()

	return p, nil
}

func (c *Context) removePlayer(p *Player) {
	c.mu.Lock()
	delete(c.players, p)
	c.mu.Unlock()
}

// Close releases all resources. All Players are closed.
func (c *Context) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true

	// Copy slice to avoid holding lock during close
	players := make([]*Player, 0, len(c.players))
	for p := range c.players {
		players = append(players, p)
	}
	c.players = nil
	c.mu.Unlock()

	for _, p := range players {
		p.Close()
	}

	return nil
}
