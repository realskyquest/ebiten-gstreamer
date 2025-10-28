package gstreamer

import (
	"errors"
	"image"
	"image/color"
	"sync"

	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
	"github.com/hajimehoshi/ebiten/v2"
)

// State of the video pipeline.
type State string

// Initial state and when stopped.
const StateStopped State = "stopped"

// State when video is playing.
const StatePlaying State = "playing"

// State when video is paused.
const StatePaused State = "paused"

// Video holds the video frame and gstreamer pipeline, and manages video state.
type Video struct {
	mu          sync.RWMutex
	frame       *image.RGBA
	texture     *ebiten.Image
	pipeline    *gst.Pipeline
	state       State
	needsUpdate bool
	width       int
	height      int
}

// Indicator when a given pipeline is missing the appsink element.
var ErrNoSuchSink = errors.New("need appsink element in pipeline")

var once sync.Once

// Initializes gstreamer. Used internally, and can be used for gst pipelines prepared externally.
func Init() {
	once.Do(func() {
		gst.Init(nil)
	})
}

func newVid() *Video {
	vid := &Video{
		state:  StateStopped,
		frame:  newImageFilled(16, 9, color.Black),
		width:  16,
		height: 9,
	}
	vid.texture = ebiten.NewImageFromImage(vid.frame)
	vid.texture = ebiten.NewImageFromImageWithOptions(
		vid.frame,
		&ebiten.NewImageFromImageOptions{
			Unmanaged: true,
		},
	)
	return vid
}

// Returns a *Video with a prepared gstreamer pipeline using the given URI.
func newVideo(uri string) (*Video, error) {
	vid := newVid()

	Init()

	sink, err := gst.NewElement("appsink")
	if err != nil {
		return nil, err
	}

	vid.setupSink(sink)

	playbin, err := gst.NewElement("playbin")
	if err != nil {
		return nil, err
	}

	playbin.Set("uri", uri)
	playbin.Set("video-sink", sink)

	pipeline, err := gst.NewPipeline("playbin-pipeline")
	if err != nil {
		return nil, err
	}

	pipeline.Add(playbin)

	vid.pipeline = pipeline

	return vid, nil
}

// Returns a new *Video using the given gstreamer pipeline.
func NewVideoFromPipeline(pipeline *gst.Pipeline) (*Video, error) {
	vid := newVid()

	Init()

	if err := vid.setupSinkFromPipeline(pipeline); err != nil {
		return nil, err
	}

	vid.pipeline = pipeline

	return vid, nil
}

// Returns a new video, using gstreamer to parse the given launch string.
// Needs to include an appsink element to be used. Example: "videotestsrc ! appsink"
func NewVideoFromLaunchString(s string) (*Video, error) {
	vid := newVid()

	Init()

	pipeline, err := gst.NewPipelineFromString(s)
	if err != nil {
		return nil, err
	}

	if err := vid.setupSinkFromPipeline(pipeline); err != nil {
		return nil, err
	}

	vid.pipeline = pipeline

	return vid, nil
}

func newImageFilled(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rectangle{Max: image.Point{w, h}})
	for x := 0; x < img.Bounds().Max.X; x++ {
		for y := 0; y < img.Bounds().Max.Y; y++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func (vid *Video) setupSinkFromPipeline(pipeline *gst.Pipeline) error {
	sinks, err := pipeline.GetSinkElements()
	if err != nil {
		return err
	}

	if len(sinks) < 1 {
		return ErrNoSuchSink
	}

	vid.setupSink(sinks[0])

	return nil
}

func (vid *Video) setupSink(sink *gst.Element) {
	sink.Set("emit-signals", true)
	sink.Set("max-buffers", 1)
	sink.Set("drop", true)

	appSink := app.SinkFromElement(sink)
	appSink.SetCaps(gst.NewCapsFromString("video/x-raw,format=RGBA"))
	appSink.SetCallbacks(&app.SinkCallbacks{
		NewSampleFunc: vid.onNewSampleFunc,
	})
}

func (vid *Video) onNewSampleFunc(sink *app.Sink) gst.FlowReturn {
	sample := sink.PullSample()
	if sample == nil {
		return gst.FlowEOS
	}

	buffer := sample.GetBuffer()
	if buffer == nil {
		return gst.FlowError
	}

	caps := sample.GetCaps()
	structure := caps.GetStructureAt(0)

	width, _ := structure.GetValue("width")
	height, _ := structure.GetValue("height")
	w, _ := width.(int)
	h, _ := height.(int)

	bufmap := buffer.Map(gst.MapRead)
	defer buffer.Unmap()

	data := bufmap.Bytes()

	vid.mu.Lock()
	defer vid.mu.Unlock()

	// Recreate frame if dimensions changed
	if vid.frame == nil || !(vid.frame.Rect.Max.X == w && vid.frame.Rect.Max.Y == h) {
		vid.frame = image.NewRGBA(image.Rect(0, 0, w, h))
		vid.width = w
		vid.height = h
	}

	copy(vid.frame.Pix, data)
	vid.needsUpdate = true

	return gst.FlowOK
}

// Starts playing the prepared pipeline. Returns an error when it failed, otherwise nil.
func (vid *Video) Play() error {
	vid.mu.Lock()
	defer vid.mu.Unlock()

	if vid.pipeline == nil {
		return nil
	}

	err := vid.pipeline.SetState(gst.StatePlaying)
	if err == nil {
		vid.state = StatePlaying
	}
	return err
}

// Pauses the video, returning an error when it failed, otherwise nil.
func (vid *Video) Pause() error {
	vid.mu.Lock()
	defer vid.mu.Unlock()

	if vid.pipeline == nil {
		return nil
	}

	err := vid.pipeline.SetState(gst.StatePaused)
	if err == nil {
		vid.state = StatePaused
	}
	return err
}

// Stops the pipeline, returning an error when it failed, otherwise nil.
func (vid *Video) Stop() error {
	vid.mu.Lock()
	defer vid.mu.Unlock()

	if vid.pipeline == nil {
		return nil
	}

	err := vid.pipeline.SetState(gst.StateNull)
	if err == nil {
		vid.state = StateStopped
	}
	return err
}

// Returns the current state of the pipeline.
func (vid *Video) State() State {
	vid.mu.RLock()
	defer vid.mu.RUnlock()

	return vid.state
}

// Destroy releases gstreamer resources. Should be called when done with the video.
func (vid *Video) Destroy() {
	vid.mu.Lock()
	defer vid.mu.Unlock()

	if vid.pipeline == nil {
		return
	}

	err := vid.pipeline.SetState(gst.StateNull)
	if err == nil {
		vid.state = StateStopped
	}
	vid.pipeline = nil
}
