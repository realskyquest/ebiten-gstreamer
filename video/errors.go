package video

import "errors"

var (
	// ErrContextClosed is returned when operating on a closed Context.
	ErrContextClosed = errors.New("video: context is closed")

	// ErrPlayerClosed is returned when operating on a closed Player.
	ErrPlayerClosed = errors.New("video: player is closed")

	// ErrAlreadyPlaying is returned when Play is called on a playing Player.
	ErrAlreadyPlaying = errors.New("video: player is already playing")

	// ErrNotSeekable is returned when seeking a non-seekable source (e.g. live stream).
	ErrNotSeekable = errors.New("video: source is not seekable")
)

// PipelineError represents a GStreamer pipeline error.
type PipelineError struct {
	Src     string
	Message string
	Debug   string
}

func (e *PipelineError) Error() string {
	s := "video: pipeline error"
	if e.Src != "" {
		s += " from " + e.Src
	}
	s += ": " + e.Message
	if e.Debug != "" {
		s += " (" + e.Debug + ")"
	}
	return s
}
