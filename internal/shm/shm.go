package shm

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"unsafe"

	sidecar "github.com/realskyquest/ebiten-gstreamer/videosidecar"
)

// Shared memory layout (128-byte header + 2× frame buffers for double-buffering)
//
// Offset  Size  Field
// ──────  ────  ──────────────────
// 0       4     Magic (0x56464D53)
// 4       4     Version (1)
// 8       4     MaxWidth
// 12      4     MaxHeight
// 16      4     Width          (current frame)
// 20      4     Height         (current frame)
// 24      4     Stride         (bytes per row = Width*4)
// 28      4     FrameSize      (Stride*Height)
// 32      8     FrameNumber    (monotonic counter)
// 40      8     PtsNs          (presentation timestamp, nanoseconds)
// 48      4     ActiveIndex    (0 or 1 – which buffer is ready to read) [ATOMIC]
// 52      76    Reserved
// 128     N     FrameBuffer[0] (N = MaxWidth * MaxHeight * 4)
// 128+N   N     FrameBuffer[1]

const (
	Magic         = 0x56464D53 // "VFMS"
	Version       = 1          // shared memory version
	ShmHeaderSize = 128        // bytes per shared memory region
	BPP           = 4          // bytes per pixel (RGBA)
)

// maxFrameBytes returns the maximum frame size in bytes.
func maxFrameBytes(maxW, maxH uint32) int {
	return int(maxW) * int(maxH) * BPP
}

// TotalSize returns the required shared memory size.
func TotalSize(maxW, maxH uint32) int {
	return ShmHeaderSize + 2*maxFrameBytes(maxW, maxH)
}

// FrameHeader is the decoded header from shared memory.
type FrameHeader struct {
	Magic       uint32 // "VFMS"
	Version     uint32 // shared memory version
	MaxWidth    uint32
	MaxHeight   uint32
	Width       uint32
	Height      uint32
	Stride      uint32 // bytes per row
	FrameSize   uint32 // bytes per frame
	FrameNumber uint64 // frame number
	PtsNs       int64  // presentation timestamp, nanoseconds
	ActiveIndex uint32 // 0 or 1 – which buffer is ready to read
}

// SharedMem provides cross-platform double-buffered frame sharing.
type SharedMem struct {
	name  string // name of the shared memory region
	data  []byte // shared memory region
	size  int    // size of the shared memory region
	maxW  uint32
	maxH  uint32
	owner bool        // true if the shared memory region is owned by the sidecar
	plat  platformShm // platform-specific implementation
}

// Create allocates a new shared memory region (call from sidecar).
func Create(name string, maxW, maxH uint32) (*SharedMem, error) {
	size := TotalSize(maxW, maxH)
	s := &SharedMem{name: name, size: size, maxW: maxW, maxH: maxH, owner: true}
	if err := s.plat.create(name, size); err != nil {
		return nil, fmt.Errorf("%w: %w", sidecar.ErrShmCreate, err)
	}
	s.data = s.plat.data()

	// Zero out and write magic header
	for i := range s.data[:ShmHeaderSize] {
		s.data[i] = 0
	}
	le := binary.LittleEndian
	le.PutUint32(s.data[0:4], Magic)
	le.PutUint32(s.data[4:8], Version)
	le.PutUint32(s.data[8:12], maxW)
	le.PutUint32(s.data[12:16], maxH)
	return s, nil
}

// Open attaches to an existing shared memory region (call from client).
func Open(name string, maxW, maxH uint32) (*SharedMem, error) {
	size := TotalSize(maxW, maxH)
	s := &SharedMem{name: name, size: size, maxW: maxW, maxH: maxH, owner: false}
	if err := s.plat.open(name, size); err != nil {
		return nil, fmt.Errorf("%w: %w", sidecar.ErrShmOpen, err)
	}
	s.data = s.plat.data()

	magic := binary.LittleEndian.Uint32(s.data[0:4])
	if magic != Magic {
		_ = s.Close()
		return nil, fmt.Errorf("%w: %#x", sidecar.ErrShmBadMagic, magic)
	}
	return s, nil
}

// Close closes the shared memory region.
func (s *SharedMem) Close() error {
	return s.plat.close(s.owner)
}

// Writer side (sidecar)

// WriteFrame writes RGBA data to the inactive buffer then swaps the active index.
func (s *SharedMem) WriteFrame(width, height, stride uint32, ptsNs int64, frameNum uint64, rgba []byte) error {
	frameSize := stride * height
	maxFrame := uint32(maxFrameBytes(s.maxW, s.maxH))
	if frameSize > maxFrame {
		return fmt.Errorf("%w: %dx%d (%d bytes) - %d", sidecar.ErrShmFrameExceedsMax, width, height, frameSize, maxFrame)
	}

	// Determine write target: the buffer NOT currently active
	activePtr := (*uint32)(unsafe.Pointer(&s.data[48]))
	active := atomic.LoadUint32(activePtr)
	writeIdx := 1 - active

	// Copy pixel data into the write buffer
	offset := ShmHeaderSize + int(writeIdx)*maxFrameBytes(s.maxW, s.maxH)
	copy(s.data[offset:offset+int(frameSize)], rgba[:frameSize])

	// Update header fields (non-atomic; they are only read after active index swap)
	le := binary.LittleEndian
	le.PutUint32(s.data[16:20], width)
	le.PutUint32(s.data[20:24], height)
	le.PutUint32(s.data[24:28], stride)
	le.PutUint32(s.data[28:32], frameSize)
	le.PutUint64(s.data[32:40], frameNum)
	le.PutUint64(s.data[40:48], uint64(ptsNs))

	// Publish: atomically swap active index so reader sees the new frame
	atomic.StoreUint32(activePtr, writeIdx)
	return nil
}

// Reader side (client)

// ReadHeader returns a snapshot of the current header (lock-free).
func (s *SharedMem) ReadHeader() FrameHeader {
	le := binary.LittleEndian
	activePtr := (*uint32)(unsafe.Pointer(&s.data[48]))
	return FrameHeader{
		Magic:       le.Uint32(s.data[0:4]),
		Version:     le.Uint32(s.data[4:8]),
		MaxWidth:    le.Uint32(s.data[8:12]),
		MaxHeight:   le.Uint32(s.data[12:16]),
		Width:       le.Uint32(s.data[16:20]),
		Height:      le.Uint32(s.data[20:24]),
		Stride:      le.Uint32(s.data[24:28]),
		FrameSize:   le.Uint32(s.data[28:32]),
		FrameNumber: le.Uint64(s.data[32:40]),
		PtsNs:       int64(le.Uint64(s.data[40:48])),
		ActiveIndex: atomic.LoadUint32(activePtr),
	}
}

// ReadFrame copies the active frame into dst. Returns dimensions and whether
// a new frame was available (compared to prevFrameNum).
func (s *SharedMem) ReadFrame(dst []byte, prevFrameNum uint64) (hdr FrameHeader, isNew bool) {
	hdr = s.ReadHeader()
	if hdr.FrameNumber == prevFrameNum || hdr.FrameSize == 0 {
		return hdr, false
	}

	offset := ShmHeaderSize + int(hdr.ActiveIndex)*maxFrameBytes(s.maxW, s.maxH)
	n := min(int(hdr.FrameSize), len(dst))
	copy(dst[:n], s.data[offset:offset+n])
	return hdr, true
}
