package shm

import (
	"testing"
)

func TestWriteReadRoundtrip(t *testing.T) {
	const maxW, maxH uint32 = 64, 64

	writer, err := Create("test_roundtrip", maxW, maxH)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer writer.Close()

	reader, err := Open("test_roundtrip", maxW, maxH)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer reader.Close()

	// Write a frame
	pixels := make([]byte, 32*32*4)
	for i := range pixels {
		pixels[i] = byte(i % 256)
	}
	if err := writer.WriteFrame(32, 32, 32*4, 123456, 1, pixels); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	// Read it back
	dst := make([]byte, int(maxW)*int(maxH)*4)
	hdr, isNew := reader.ReadFrame(dst, 0)
	if !isNew {
		t.Fatal("ReadFrame: expected isNew=true")
	}
	if hdr.FrameNumber != 1 {
		t.Errorf("FrameNumber = %d, want 1", hdr.FrameNumber)
	}
	if hdr.Width != 32 || hdr.Height != 32 {
		t.Errorf("dimensions = %dx%d, want 32x32", hdr.Width, hdr.Height)
	}
	for i := 0; i < 32*32*4; i++ {
		if dst[i] != pixels[i] {
			t.Fatalf("pixel[%d] = %d, want %d", i, dst[i], pixels[i])
		}
	}

	// Second read with same frame number should return isNew=false
	_, isNew = reader.ReadFrame(dst, hdr.FrameNumber)
	if isNew {
		t.Error("ReadFrame with same frameNum: expected isNew=false")
	}
}

func TestBadMagic(t *testing.T) {
	writer, err := Create("test_bad_magic", 64, 64)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writer.data[0] = 0xFF // corrupt magic
	writer.Close()

	_, err = Open("test_bad_magic", 64, 64)
	if err == nil {
		t.Fatal("Open with corrupted magic: expected error")
	}
}
