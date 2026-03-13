// Package video provides GStreamer-backed video playback for Ebitengine.
//
// # Quick Start
//
//	ctx, err := video.NewContext()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	player, err := ctx.NewPlayer("https://example.com/video.mp4")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	player.Play()
//
//	// In your Ebitengine Draw():
//	func (g *Game) Draw(screen *ebiten.Image) {
//	    if f := player.Frame(); f != nil {
//	        screen.DrawImage(f, nil)
//	    }
//	}
//
//	// When done:
//	player.Close()
//	ctx.Close()
//
// Each Player owns its own GStreamer pipeline and decode goroutine.
// Multiple Players can run simultaneously.
package video
