// MIT License
//
// Copyright (c) 2025 realskyquest & Consult With Simon
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"fmt"
	"log"
	"os"
	"sync"

	gstreamer "github.com/realskyquest/ebiten-gstreamer" // Replace with your actual package path

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// mpgURL is a URL of an example MPEG-1 video. The license is the following:
//
// https://commons.wikimedia.org/wiki/File:Shibuya_Crossing,_Tokyo,_Japan_(video).webm
// "Shibuya Crossing, Tokyo, Japan (video).webm" by Basile Morin
// The Creative Commons Attribution-Share Alike 4.0 International license
const mpgURL = "https://example-resources.ebitengine.org/shibuya.mpg"

type Game struct {
	video *gstreamer.Video
	sync  sync.Once
}

func (g *Game) Update() error {
	g.sync.Do(func() {
		g.video.Play()
	})

	// Update video texture
	return g.video.Update()
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Draw video centered on screen
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	vw, vh := g.video.Size()

	if vw > 0 && vh > 0 {
		opts := &ebiten.DrawImageOptions{}

		// IMPORTANT: Use FilterLinear for better quality
		opts.Filter = ebiten.FilterLinear

		// Calculate scale to fit screen while maintaining aspect ratio
		scale := min(float64(sw)/float64(vw), float64(sh)/float64(vh))

		// For pixel-perfect display at 1:1 scale, use integer scaling
		// This prevents sub-pixel rendering which can cause quality loss
		if scale >= 1.0 {
			// When scaling up, you might want to use integer scaling for pixel art
			// scale = float64(int(scale))
		}

		opts.GeoM.Scale(scale, scale)

		// Center the video
		opts.GeoM.Translate(
			float64(sw-int(float64(vw)*scale))/2,
			float64(sh-int(float64(vh)*scale))/2,
		)

		g.video.Draw(screen, opts)
	}

	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f", ebiten.ActualFPS()))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	// Create video from file URI
	var videoPath string

	if len(os.Args) > 1 {
		videoPath = os.Args[1]
	} else {
		videoPath = mpgURL
		fmt.Println("Play the default video. You can specify a video file as an argument.")
	}

	video, err := gstreamer.NewVideo(videoPath)
	if err != nil {
		log.Fatal(err)
	}
	defer video.Destroy()

	game := &Game{video: video}

	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("Video Player")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	ebiten.SetVsyncEnabled(true)

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
