package gstreamer

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// -- Extra for video --

// NewVideoFromPath creates a *Video from a URL (http/https/rtsp/etc) or a local file path.
func NewVideo(uri string) (*Video, error) {

	u, err := url.Parse(uri)
	if err == nil && u.Scheme != "" {
		uri = uri
	} else {
		absPath, err := filepath.Abs(uri)
		if err != nil {
			return nil, err
		}

		if runtime.GOOS == "windows" {
			// Windows: file:///C:/path/to/file
			uri = "file:///" + strings.ReplaceAll(absPath, "\\", "/")
		} else {
			// Unix-like: file:///path/to/file
			uri = "file://" + absPath
		}
	}

	return newVideo(uri)
}

// -- End of extra --

// Update should be called in ebitengine's Update function to update video texture.
func (vid *Video) Update() error {
	vid.mu.Lock()
	defer vid.mu.Unlock()

	if vid.needsUpdate && vid.frame != nil {
		// Recreate texture if dimensions changed
		if vid.texture == nil || vid.texture.Bounds().Dx() != vid.width || vid.texture.Bounds().Dy() != vid.height {
			vid.texture = ebiten.NewImage(vid.width, vid.height)
		}
		vid.texture.WritePixels(vid.frame.Pix)
		vid.needsUpdate = false
	}

	return nil
}

// Draw draws the video frame with DrawImageOptions.
func (vid *Video) Draw(screen *ebiten.Image, options *ebiten.DrawImageOptions) {
	vid.mu.RLock()
	defer vid.mu.RUnlock()

	screen.DrawImage(vid.texture, options)
}

// GetTexture returns the current video texture for custom drawing.
func (vid *Video) GetTexture() *ebiten.Image {
	vid.mu.RLock()
	defer vid.mu.RUnlock()
	return vid.texture
}

// Size returns the current video dimensions.
func (vid *Video) Size() (int, int) {
	vid.mu.RLock()
	defer vid.mu.RUnlock()
	return vid.width, vid.height
}
