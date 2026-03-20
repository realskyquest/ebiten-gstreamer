package video

import (
	"runtime"
	"testing"
)

func TestToURI_HTTP(t *testing.T) {
	for _, u := range []string{
		"http://example.com/v.mp4",
		"https://example.com/v.mp4",
		"rtsp://cam.local/stream",
		"rtsps://cam.local/stream",
	} {
		if got := toURI(u); got != u {
			t.Errorf("toURI(%q) = %q, want passthrough", u, got)
		}
	}
}

func TestToURI_FileURI(t *testing.T) {
	u := "file:///home/user/video.mp4"
	if got := toURI(u); got != u {
		t.Errorf("toURI(%q) = %q, want passthrough", u, got)
	}
}

func TestToURI_RelativePath(t *testing.T) {
	got := toURI("video.mp4")
	if runtime.GOOS == "windows" {
		if len(got) < 10 || got[:8] != "file:///" {
			t.Errorf("toURI on Windows = %q, want file:///... prefix", got)
		}
	} else {
		if len(got) < 7 || got[:7] != "file://" {
			t.Errorf("toURI = %q, want file:// prefix", got)
		}
	}
}

func TestToURI_AbsolutePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("absolute path format differs on Windows")
	}
	got := toURI("/home/user/video.mp4")
	want := "file:///home/user/video.mp4"
	if got != want {
		t.Errorf("toURI(%q) = %q, want %q", "/home/user/video.mp4", got, want)
	}
}
