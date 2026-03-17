package videoutils

import "errors"

var (
	// ErrOpenFailed is returned when opening a file fails.
	ErrOpenFailed = errors.New("videoutils: failed to open file")

	// ErrTempFile is returned when creating a temp file fails.
	ErrTempFile = errors.New("videoutils: failed to create temp file")

	// ErrCopyFailed is returned when copying a file fails.
	ErrCopyFailed = errors.New("videoutils: failed to copy file")
)
