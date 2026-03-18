package sidecar

import "errors"

var (
	// ErrProtocolPayloadTooLarge is returned when a message payload is too large.
	ErrProtocolPayloadTooLarge = errors.New("protocol: payload too large")

	// ErrProtocolMarshal is returned when marshaling a message fails.
	ErrProtocolMarshal = errors.New("protocol: marshal")

	// ErrProtocolWriteHeader is returned when writing a message header fails.
	ErrProtocolWriteHeader = errors.New("protocol: write header")

	// ErrProtocolWriteBody is returned when writing a message body fails.
	ErrProtocolWriteBody = errors.New("protocol: write body")

	// ErrProtocolReadHeader is returned when reading a message header fails.
	ErrProtocolReadHeader = errors.New("protocol: read header")

	// ErrProtocolReadBody is returned when reading a message body fails.
	ErrProtocolReadBody = errors.New("protocol: read body")
)

var (
	// ErrShmBadMagic is returned when the shared memory region has an invalid magic number.
	ErrShmBadMagic = errors.New("shm: bad magic")

	// ErrShmFrameExceedsMax is returned when a frame exceeds the maximum size.
	ErrShmFrameExceedsMax = errors.New("shm: frame exceeds max")

	// ErrShmCreate is returned when creating a shared memory region fails.
	ErrShmCreate = errors.New("shm: create")

	// ErrShmOpen is returned when opening an existing shared memory region fails.
	ErrShmOpen = errors.New("shm: open")
)
