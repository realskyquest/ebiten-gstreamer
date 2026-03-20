package shm

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
	"unsafe"

	sidecar "github.com/realskyquest/ebiten-gstreamer/videosidecar"
)

// Shared memory layout
//
// ── Global header (128 bytes, fixed) ──────────────────────────────────────────
//
//	Offset  Size  Field
//	──────  ────  ──────────────────────────────────────────────
//	0       4     Magic (0x56464D53 "VFMS")
//	4       4     Version (2)
//	8       4     MaxWidth
//	12      4     MaxHeight
//	16      32    Reserved (was per-frame metadata in v1)
//	48      4     ActiveIndex (0 or 1) [ATOMIC]
//	52      76    Reserved
//
// ── Per-slot block (repeated twice, once per double-buffer slot) ───────────────
//
//	Offset from slot start  Size  Field
//	──────────────────────  ────  ──────────────────────────────
//	0                       4     Width  (pixels)
//	4                       4     Height (pixels)
//	8                       4     Stride (bytes per row = Width*4)
//	12                      4     FrameSize (Stride*Height)
//	16                      8     FrameNumber (monotonic; 0 = no frame yet)
//	24                      8     PtsNs (presentation timestamp, nanoseconds)
//	32                      N     RGBA pixel data  (N = MaxWidth*MaxHeight*4)
//
// Total size: ShmHeaderSize + 2*(SlotHeaderSize + MaxWidth*MaxHeight*4)

const (
	Magic          = 0x56464D53 // "VFMS"
	Version        = 2          // v2: per-slot metadata (v1 used shared global fields)
	ShmHeaderSize  = 128        // global header, bytes
	SlotHeaderSize = 32         // per-slot metadata, bytes
	BPP            = 4          // bytes per pixel (RGBA)
)

// slotStride returns the total byte size of one slot (header + pixel buffer).
func slotStride(maxW, maxH uint32) int {
	return SlotHeaderSize + int(maxW)*int(maxH)*BPP
}

// slotBase returns the byte offset within s.data of slot idx.
func slotBase(maxW, maxH uint32, idx uint32) int {
	return ShmHeaderSize + int(idx)*slotStride(maxW, maxH)
}

// TotalSize returns the required shared memory size for the given max dimensions.
// Callers (platform files, client) use this to allocate/map the region.
func TotalSize(maxW, maxH uint32) int {
	return ShmHeaderSize + 2*slotStride(maxW, maxH)
}

// FrameHeader is a decoded snapshot of one slot's metadata.
type FrameHeader struct {
	Width       uint32
	Height      uint32
	Stride      uint32 // bytes per row
	FrameSize   uint32 // total pixel bytes (Stride * Height)
	FrameNumber uint64 // monotonic counter; 0 means no frame written yet
	PtsNs       int64  // presentation timestamp, nanoseconds
	ActiveIndex uint32 // which slot index this header was read from
}

// SharedMem provides cross-platform double-buffered RGBA frame sharing.
type SharedMem struct {
	name  string
	data  []byte
	size  int
	maxW  uint32
	maxH  uint32
	owner bool        // true if this instance created (and must clean up) the region
	plat  platformShm // platform-specific mmap/handle
}

// Create allocates a new shared memory region. Called from the sidecar process.
func Create(name string, maxW, maxH uint32) (*SharedMem, error) {
	size := TotalSize(maxW, maxH)
	s := &SharedMem{name: name, size: size, maxW: maxW, maxH: maxH, owner: true}
	if err := s.plat.create(name, size); err != nil {
		return nil, fmt.Errorf("%w: %w", sidecar.ErrShmCreate, err)
	}
	s.data = s.plat.data()

	// Zero the entire region, then stamp the global header.
	for i := range s.data {
		s.data[i] = 0
	}
	le := binary.LittleEndian
	le.PutUint32(s.data[0:4], Magic)
	le.PutUint32(s.data[4:8], Version)
	le.PutUint32(s.data[8:12], maxW)
	le.PutUint32(s.data[12:16], maxH)
	// ActiveIndex at offset 48 is already 0 (zeroed above).
	// Slot headers are also zeroed; FrameNumber=0 signals "no frame yet".
	return s, nil
}

// Open attaches to an existing shared memory region. Called from the client process.
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

// Close unmaps and optionally unlinks the shared memory region.
func (s *SharedMem) Close() error {
	return s.plat.close(s.owner)
}

// Writer side (sidecar)

// WriteFrame writes one decoded RGBA frame into the inactive slot and then
// atomically publishes it by swapping the active index.
//
// Write order within a slot:
//  1. Pixel data  — written first, before the slot header.
//  2. Slot header — width/height/stride/frameNum/ptsNs for these exact pixels.
//  3. atomic.StoreUint32(activeIndex, writeIdx) — the release barrier.
//
// The reader cannot see the new slot until step 3 completes. At that point both
// pixel data and slot header are fully committed, so the reader always observes
// a consistent pair.
//
// Note: pixels are written BEFORE the slot header intentionally. If a reader
// somehow observed a stale activeIndex and landed on this slot while writing was
// still in progress (impossible with a correct atomic publish, but defensive),
// it would see FrameNumber=prev (still from the last publish) and skip the frame
// as not-new. Only after the slot header is written and activeIndex is published
// will any reader act on the new frame.
func (s *SharedMem) WriteFrame(width, height, stride uint32, ptsNs int64, frameNum uint64, rgba []byte) error {
	frameSize := stride * height
	maxFrame := uint32(int(s.maxW) * int(s.maxH) * BPP)
	if frameSize > maxFrame {
		return fmt.Errorf("%w: %dx%d (%d bytes), max %d",
			sidecar.ErrShmFrameExceedsMax, width, height, frameSize, maxFrame)
	}

	// Choose the slot that is NOT currently active (the inactive/back buffer).
	activePtr := (*uint32)(unsafe.Pointer(&s.data[48]))
	writeIdx := 1 - atomic.LoadUint32(activePtr)

	base := slotBase(s.maxW, s.maxH, writeIdx)

	// 1. Write pixel data into the slot's pixel region (after the 32-byte slot header).
	pixStart := base + SlotHeaderSize
	copy(s.data[pixStart:pixStart+int(frameSize)], rgba[:frameSize])

	// 2. Write the slot header — metadata that describes these exact pixels.
	//    Using LittleEndian non-atomically is fine: the atomic store below acts
	//    as a release barrier, ensuring these writes are globally visible before
	//    any reader observes the updated activeIndex.
	le := binary.LittleEndian
	le.PutUint32(s.data[base+0:], width)
	le.PutUint32(s.data[base+4:], height)
	le.PutUint32(s.data[base+8:], stride)
	le.PutUint32(s.data[base+12:], frameSize)
	le.PutUint64(s.data[base+16:], frameNum)
	le.PutUint64(s.data[base+24:], uint64(ptsNs))

	// 3. Publish: atomic store is the release point.
	//    Readers that load activeIndex after this store will see writeIdx and
	//    therefore the fully-written slot header and pixel data above.
	atomic.StoreUint32(activePtr, writeIdx)
	return nil
}

// Reader side (client)

// ReadFrame copies the latest decoded frame into dst and returns its metadata.
// Returns isNew=false if no new frame is available (same FrameNumber as last call).
//
// Read order:
//  1. atomic.LoadUint32(activeIndex) — single acquire load; no other loads can
//     be reordered before this by the CPU or compiler.
//  2. Read slot[idx] header — width, height, stride, frameNum, ptsNs.
//     These fields live in the same slot as the pixels and were written before
//     the atomic publish, so they are always consistent with the pixels below.
//  3. Check FrameNumber — skip if equal to prevFrameNum (no new frame).
//  4. Copy slot[idx] pixels into dst.
//
// There is no window between steps 1 and 2 where the writer can invalidate
// the metadata: the writer always targets the *other* slot (1-activeIndex), so
// the slot we are reading cannot change until after we next call ReadFrame and
// the writer has had a chance to fill the other slot and publish again.
func (s *SharedMem) ReadFrame(dst []byte, prevFrameNum uint64) (hdr FrameHeader, isNew bool) {
	// Step 1: single atomic acquire load.
	activePtr := (*uint32)(unsafe.Pointer(&s.data[48]))
	idx := atomic.LoadUint32(activePtr)

	// Step 2: read this slot's header (always consistent with its pixels).
	base := slotBase(s.maxW, s.maxH, idx)
	le := binary.LittleEndian
	hdr = FrameHeader{
		Width:       le.Uint32(s.data[base+0:]),
		Height:      le.Uint32(s.data[base+4:]),
		Stride:      le.Uint32(s.data[base+8:]),
		FrameSize:   le.Uint32(s.data[base+12:]),
		FrameNumber: le.Uint64(s.data[base+16:]),
		PtsNs:       int64(le.Uint64(s.data[base+24:])),
		ActiveIndex: idx,
	}

	// Step 3: nothing new to return.
	if hdr.FrameNumber == prevFrameNum || hdr.FrameSize == 0 {
		return hdr, false
	}

	// Step 4: copy pixels from this slot's pixel region.
	pixStart := base + SlotHeaderSize
	n := min(int(hdr.FrameSize), len(dst))
	copy(dst[:n], s.data[pixStart:pixStart+n])
	return hdr, true
}
