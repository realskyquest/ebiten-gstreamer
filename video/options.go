package video

// PlayerOptions configures a Player at creation time.
type PlayerOptions struct {
	// Loop makes the video restart from the beginning when it reaches the end.
	Loop bool

	// Rate sets the playback rate. 1.0 is normal, 2.0 is double speed.
	// Negative values play in reverse (if the source supports it).
	Rate float64

	// Volume sets the initial volume. Range: 0.0 (muted) to 1.0 (full). Default: 1.0.
	Volume float64

	// Muted mutes the video. Default: false.
	Muted bool

	// MaxBufferedFrames controls the appsink queue depth.
	// Higher values increase memory usage but smooth over decode stalls.
	// 0 means use default (2).
	MaxBufferedFrames uint

	// Width forces the output video width. 0 means use source width.
	Width int

	// Height forces the output video height. 0 means use source height.
	Height int

	// OnEnd is called when the video reaches EOS (after looping is considered).
	// Called from a GStreamer goroutine — keep it fast and non-blocking.
	OnEnd func()

	// OnError is called when the GStreamer pipeline encounters an error.
	// Called from a GStreamer goroutine.
	OnError func(err error)

	// OnBuffering is called with the buffering percentage (0-100) for network sources.
	// Called from a GStreamer goroutine.
	OnBuffering func(percent int)
}

func (o *PlayerOptions) defaults() {
	if o.Volume == 0 && !o.Muted {
		o.Volume = 1.0
	}
	if o.Rate == 0 {
		o.Rate = 1.0
	}
	if o.MaxBufferedFrames == 0 {
		o.MaxBufferedFrames = 2
	}
}
