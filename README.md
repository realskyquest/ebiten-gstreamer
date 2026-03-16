# Ebiten-GStreamer

GStreamer-backed video playback for [Ebitengine](https://ebitengine.org/). Play video files and streams in your 2D Go games with hardware-accelerated decoding.

## Features

- **Multiple source support**: Local files, HTTP(S) URLs, RTSP streams, and any URI GStreamer supports
- **Hardware acceleration**: Leverages GStreamer's hardware-accelerated decoding when available
- **Concurrent playback**: Multiple independent video players can run simultaneously
- **Frame-by-frame control**: Play, pause, seek, and retrieve current video frames as `*ebiten.Image`
- **Customizable**: Volume control, looping, video scaling, and buffering options
- **Event callbacks**: OnEnd, OnError, and OnBuffering hooks for responsive UI
- **Cross-platform**: Works on Linux, macOS, Windows (requires GStreamer), and Web

## Requirements

### GStreamer (Native platforms only)

On **Linux, macOS, and Windows**, GStreamer must be installed on the user's device with the necessary plugins. The web build does not require GStreamer or any native dependencies, playback is handled entirely by the browser.

Install GStreamer for your platform: https://gstreamer.freedesktop.org/documentation/installing

> **Web builds** (`GOOS=js GOARCH=wasm`) have **no GStreamer requirement**. Video is decoded natively by the browser via an HTML5 `<video>` element.

## Installation

```bash
go get github.com/realskyquest/ebiten-gstreamer/video
```

## Building

There are three backends, selected at build time. The public API is identical across all of them.

### Native (CGo + GStreamer)

The default build. Uses GStreamer directly via CGo bindings ([go-gst](https://github.com/go-gst/go-gst)). Requires GStreamer installed on the end user's machine.

```bash
go build ./...
```

**Requires:** GStreamer on the host at runtime. CGo toolchain at build time.

---

### Web (HTML5 `<video>`)

Targets `js/wasm` via `GOOS=js GOARCH=wasm`. No GStreamer, no CGo, no native dependencies of any kind. Decoding is handled by the browser.

```bash
GOOS=js GOARCH=wasm go build ./...
```

**Requires:** Nothing, the browser handles everything.

---

### Sidecar (Pure Go / no CGo)

Uses a separate **sidecar process** as the GStreamer backend, communicating over TCP loopback (control messages) and shared memory (RGBA frames). Your main application binary has **zero CGo**.

```bash
go build -tags sidecar ./...
```

The sidecar binary (which contains GStreamer + CGo) must be built separately and shipped alongside your application:

```bash
# Build the sidecar helper (this binary is the only one that links GStreamer)
go build ./cmd/sidecar
```

**Requires:** GStreamer on the end user's machine (same as native). The sidecar binary must be present at runtime next to your application.

> **Why use sidecar?** This build mode is useful when you need a CGo-free application binary with fast build times without any CGo side-effects.

---

### Summary

| Build | Command | CGo in app binary | GStreamer required | Web |
|---|---|---|---|---|
| Native | `go build` | ✅ Yes | ✅ Yes | ❌ |
| Web | `GOOS=js GOARCH=wasm go build` | ❌ No | ❌ No | ✅ |
| Sidecar | `go build -tags sidecar` | ❌ No | ✅ Yes (sidecar) | ❌ |

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

The same code compiles and runs correctly under all three backends, no changes needed.

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

### Native (CGo + GStreamer)

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
<video src="."> → drawImage() → <canvas> → getImageData() → ebiten.Image
```
- The browser handles all decoding natively, no goroutines or pipelines
- Every call to `Frame()` draws the current video frame onto the canvas and reads back the raw RGBA pixels via `getImageData`
- Audio is handled directly by the browser through the `<video>` element
- No GStreamer, no CGo, no native dependencies of any kind

### Sidecar

The application binary is pure Go with no CGo. A separate sidecar process owns all GStreamer and CGo code. The two processes communicate via:

```
[App binary (pure Go)] ─── TCP loopback ──→ [Sidecar (GStreamer + CGo)]
                       ←── shared memory ─── (RGBA frames, zero-copy)
```

- **TCP loopback** carries control messages (play, pause, seek, volume, etc.) and state updates (position, duration, buffering)
- **Shared memory** carries decoded RGBA frames with zero copy, no data is transferred over the network socket for video data
- The sidecar runs the same GStreamer pipeline as the native

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Acknowledgments

- [Ebitengine](https://ebitengine.org/) - The 2D game engine
- [GStreamer](https://gstreamer.freedesktop.org/) - Multimedia framework
- [go-gst](https://github.com/go-gst/go-gst) - Go bindings for GStreamer
