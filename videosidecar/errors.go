package videosidecar

import "errors"

// internal/protocol errors
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

// internal/shm errors
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

// internal/client errors
var (
	// ErrClientStdoutPipe is returned when getting a stdout pipe fails.
	ErrClientStdoutPipe = errors.New("client: failed to get stdout pipe")

	// ErrClientSidecarReportPort is returned when the sidecar did not report a port.
	ErrClientSidecarReportPort = errors.New("client: sidecar did not report port")

	// ErrClientSidecarStart is returned when starting the sidecar process fails.
	ErrClientSidecarStart = errors.New("client: failed to start sidecar")

	// ErrClientSidecarConnect is returned when connecting to the sidecar fails.
	ErrClientSidecarConnect = errors.New("client: failed to connect to sidecar")

	// ErrClientSidecarReady is returned when the sidecar did not report ready.
	ErrClientSidecarReady = errors.New("client: sidecar did not report ready")

	// ErrClientSidecarShmOpen is returned when opening the shared memory region fails.
	ErrClientSidecarShmOpen = errors.New("client: failed to open shm")

	// ErrClientEventRead is returned when reading an event fails.
	ErrClientEventRead = errors.New("client: failed to read event")
)
