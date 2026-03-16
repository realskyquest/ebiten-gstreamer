//go:build sidecar && !js

package video

// initContext is a no-op in sidecar mode: GStreamer is initialised inside the
// sidecar process, not in the host process.
func initContext(_ *Context) error {
	return nil
}
