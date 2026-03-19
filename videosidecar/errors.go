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

// sidecar errors
var (
	// ErrSidecarShmCreate is returned when creating the shared memory region fails.
	ErrSidecarShmCreate = errors.New("sidecar: failed to create shm")

	// ErrSidecarListen is returned when the sidecar fails to listen on a port.
	ErrSidecarListen = errors.New("sidecar: listen failed")

	// ErrSidecarAccept is returned when the sidecar fails to accept a connection.
	ErrSidecarAccept = errors.New("sidecar: accept failed")

	// ErrSidecarPipeline is returned when creating the GStreamer pipeline fails.
	ErrSidecarPipeline = errors.New("sidecar: create pipeline")

	// ErrSidecarUridecodebin is returned when creating uridecodebin fails.
	ErrSidecarUridecodebin = errors.New("sidecar: uridecodebin")

	// ErrSidecarSetURI is returned when setting the URI property fails.
	ErrSidecarSetURI = errors.New("sidecar: set uri")

	// ErrSidecarVideoconvert is returned when creating videoconvert fails.
	ErrSidecarVideoconvert = errors.New("sidecar: videoconvert")

	// ErrSidecarVideoscale is returned when creating videoscale fails.
	ErrSidecarVideoscale = errors.New("sidecar: videoscale")

	// ErrSidecarCapsfilter is returned when creating capsfilter fails.
	ErrSidecarCapsfilter = errors.New("sidecar: capsfilter")

	// ErrSidecarSetCaps is returned when setting the caps property fails.
	ErrSidecarSetCaps = errors.New("sidecar: set caps")

	// ErrSidecarAppsink is returned when creating appsink fails.
	ErrSidecarAppsink = errors.New("sidecar: appsink")

	// ErrSidecarAudioconvert is returned when creating audioconvert fails.
	ErrSidecarAudioconvert = errors.New("sidecar: audioconvert")

	// ErrSidecarAudioresample is returned when creating audioresample fails.
	ErrSidecarAudioresample = errors.New("sidecar: audioresample")

	// ErrSidecarVolume is returned when creating volume element fails.
	ErrSidecarVolume = errors.New("sidecar: volume")

	// ErrSidecarSetVolume is returned when setting the volume property fails.
	ErrSidecarSetVolume = errors.New("sidecar: set volume")

	// ErrSidecarAutoaudiosink is returned when creating autoaudiosink fails.
	ErrSidecarAutoaudiosink = errors.New("sidecar: autoaudiosink")

	// ErrSidecarLinkVideo is returned when linking the video branch fails.
	ErrSidecarLinkVideo = errors.New("sidecar: link video branch")

	// ErrSidecarLinkAudio is returned when linking the audio branch fails.
	ErrSidecarLinkAudio = errors.New("sidecar: link audio branch")

	// ErrSidecarSeekFailed is returned when a seek operation fails.
	ErrSidecarSeekFailed = errors.New("sidecar: seek failed")
)
