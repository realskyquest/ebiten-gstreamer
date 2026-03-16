package video

import (
	"net/url"
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
	for _, scheme := range []string{"http://", "https://", "rtsp://", "rtsps://"} {
		if strings.HasPrefix(lower, scheme) {
			return source
		}
	}

	// Already a file URI, pass through as-is.
	if strings.HasPrefix(lower, "file://") {
		return source
	}

	// Treat as file path, resolve to absolute
	absPath, err := filepath.Abs(source)
	if err != nil {
		absPath = source
	}

	// On Windows, filepath.Abs gives C:\foo\bar, we need file:///C:/foo/bar
	if runtime.GOOS == "windows" {
		absPath = strings.ReplaceAll(absPath, `\`, "/")
		return "file:///" + absPath
	}

	u := &url.URL{
		Scheme: "file",
		Path:   absPath,
	}
	return u.String()
}
