# Ebiten-GStreamer

GStreamer-backed video playback for [Ebitengine](https://ebitengine.org/). Play video files and streams in your 2D Go games with hardware-accelerated decoding.

## Features

- **Multiple source support**: Local files, HTTP(S) URLs, RTSP streams, and any URI GStreamer supports
- **Hardware acceleration**: Leverages GStreamer's hardware-accelerated decoding when available
- **Concurrent playback**: Multiple independent video players can run simultaneously
- **Frame-by-frame control**: Play, pause, seek, and retrieve current video frames as `*ebiten.Image`
- **Customizable**: Volume control, looping, video scaling, and buffering options
- **Event callbacks**: OnEnd, OnError, and OnBuffering hooks for responsive UI
- **Cross-platform**: Works on Linux, macOS, and Windows (requires GStreamer)

## Requirements

### GStreamer

You must have GStreamer installed on your system with the necessary plugins: https://gstreamer.freedesktop.org/documentation/installing

## Installation

```bash
go get github.com/realskyquest/ebiten-gstreamer/video
```

## Quick Start

```go
package main

import (
    "log"
    "github.com/hajimehoshi/ebiten/v2"
    "github.com/realskyquest/ebiten-gstreamer/video"
)

type Game struct {
    player *video.Player
}

func (g *Game) Draw(screen *ebiten.Image) {
    if frame := g.player.Frame(); frame != nil {
        screen.DrawImage(frame, nil)
    }
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
    return outsideWidth, outsideHeight
}

func main() {
    ctx, err := video.NewContext()
    if err != nil {
        log.Fatal(err)
    }
    defer ctx.Close()

    player, err := ctx.NewPlayer("video.mp4", &video.PlayerOptions{
        Volume: 1.0,
        OnEnd: func() {
            log.Println("Video finished")
        },
        OnError: func(err error) {
            log.Println("Error:", err)
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    player.Play()

    ebiten.SetWindowSize(1280, 720)
    ebiten.SetWindowTitle("Video Player")

    if err := ebiten.RunGame(&Game{player: player}); err != nil {
        log.Fatal(err)
    }
}
```

## API Reference

### Context

```go
ctx, err := video.NewContext()
defer ctx.Close()
```

Only one Context can exist per application. Must be created before `ebiten.RunGame()`.

### Player

```go
player, err := ctx.NewPlayer(source string, opts *PlayerOptions)
```

**Source formats:**
- Local file: `"./video.mp4"`, `"/path/to/video.mp4"`
- File URI: `"file:///path/to/video.mp4"`
- HTTP(S): `"https://example.com/video.mp4"`
- RTSP: `"rtsp://camera.local/stream"`

### Thread Safety

All Player methods are safe for concurrent use. The `Frame()` method can be called from the Draw loop while other goroutines control playback.

## Examples

### Basic Player

See [`examples/basic/`](examples/basic/main.go) for a minimal video player that loads a file from command line arguments.

### Full-Featured Player

See [`examples/controls/`](examples/controls/main.go) for a complete video player with:
- Playback controls (play/pause/stop)
- Seek forward/backward
- Volume control with mute toggle
- Drag-and-drop file loading
- Progress bar and time display
- Keyboard shortcuts
- Toast notifications

Run it:
```bash
go run ./examples/controls video.mp4
```

## How It Works

### Native (GStreamer)
Each `Player` creates its own GStreamer pipeline:
```
uridecodebin → videoconvert → videoscale → capsfilter → appsink
```
- Decoding happens in a dedicated goroutine
- Frames are double-buffered for thread-safe access
- The `appsink` pushes decoded video frames to Ebiten
- Audio is automatically handled by GStreamer's default audio sink

### Web (HTML5)
Each `Player` creates a hidden `<video>` element and an offscreen `<canvas>`:
```
<video src="blob://..."> → drawImage() → <canvas> → getImageData() → ebiten.Image
```
- The browser handles all decoding natively — no goroutines or pipelines
- Every call to `Frame()` draws the current video frame onto the canvas and reads back the raw RGBA pixels via `getImageData`
- Audio is handled directly by the browser through the `<video>` element

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Acknowledgments

- [Ebitengine](https://ebitengine.org/) - The 2D game engine
- [GStreamer](https://gstreamer.freedesktop.org/) - Multimedia framework
- [go-gst](https://github.com/go-gst/go-gst) - Go bindings for GStreamer