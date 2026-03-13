package video

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// toURI converts a source string to a URI that GStreamer's uridecodebin understands.
// Accepts:
//   - URIs: http://, https://, rtsp://, rtsps://, file://
//   - Absolute file paths: /home/user/video.mp4, C:\video.mp4
//   - Relative file paths: ./video.mp4, ../assets/video.mp4
func toURI(source string) string {
	// Already a URI scheme GStreamer understands
	lower := strings.ToLower(source)
	for _, scheme := range []string{"http://", "https://", "rtsp://", "rtsps://", "file://"} {
		if strings.HasPrefix(lower, scheme) {
			return source
		}
	}

	// Treat as file path — resolve to absolute
	absPath, err := filepath.Abs(source)
	if err != nil {
		// Fallback: just prefix file://
		absPath = source
	}

	// On Windows, filepath.Abs gives C:\foo\bar, we need file:///C:/foo/bar
	if runtime.GOOS == "windows" {
		absPath = strings.ReplaceAll(absPath, `\`, "/")
	}

	u := &url.URL{
		Scheme: "file",
		Path:   absPath,
	}
	return u.String()
}

// sourceExists returns true if the source looks like a local file and exists.
// Returns true for non-file URIs (network sources can't be pre-validated).
func sourceExists(source string) bool {
	lower := strings.ToLower(source)
	for _, scheme := range []string{"http://", "https://", "rtsp://", "rtsps://"} {
		if strings.HasPrefix(lower, scheme) {
			return true // network — can't pre-check
		}
	}

	// For file:// URIs, extract the path
	path := source
	if strings.HasPrefix(lower, "file://") {
		u, err := url.Parse(source)
		if err != nil {
			return false
		}
		path = u.Path
	}

	// Resolve relative paths
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	_, err = os.Stat(abs)
	return err == nil
}
