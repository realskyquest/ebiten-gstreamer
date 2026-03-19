package videosidecar

// Error messages for user-facing errors (sent to client via EvtError)
// These can be localized or customized without recompiling the sidecar.
const (
	MsgCreatePipeline  = "create pipeline"
	MsgUridecodebin    = "uridecodebin"
	MsgSetURI          = "set uri"
	MsgVideoconvert    = "videoconvert"
	MsgVideoscale      = "videoscale"
	MsgCapsfilter      = "capsfilter"
	MsgSetCaps         = "set caps"
	MsgAppsink         = "appsink"
	MsgAudioconvert    = "audioconvert"
	MsgAudioresample   = "audioresample"
	MsgVolume          = "volume"
	MsgSetVolume       = "set volume"
	MsgAutoaudiosink   = "autoaudiosink"
	MsgLinkVideoBranch = "link video branch"
	MsgLinkAudioBranch = "link audio branch"
	MsgSeekFailed      = "seek failed: source is not seekable"
)
