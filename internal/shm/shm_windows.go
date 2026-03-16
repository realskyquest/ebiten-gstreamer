//go:build windows

package shm

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modkernel32           = syscall.NewLazyDLL("kernel32.dll")
	procCreateFileMapping = modkernel32.NewProc("CreateFileMappingW")
	procOpenFileMapping   = modkernel32.NewProc("OpenFileMappingW")
	procMapViewOfFile     = modkernel32.NewProc("MapViewOfFile")
	procUnmapViewOfFile   = modkernel32.NewProc("UnmapViewOfFile")
	procCloseHandle       = modkernel32.NewProc("CloseHandle")
)

const (
	_INVALID_HANDLE_VALUE = ^uintptr(0)
	_PAGE_READWRITE       = 0x04
	_FILE_MAP_ALL_ACCESS  = 0xF001F
)

type platformShm struct {
	handle uintptr
	addr   uintptr
	buf    []byte
	size   int
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString("Local\\vp_" + s)
	return p
}

func (p *platformShm) create(name string, size int) error {
	p.size = size
	namePtr := utf16Ptr(name)

	h, _, err := procCreateFileMapping.Call(
		_INVALID_HANDLE_VALUE,
		0, // default security
		_PAGE_READWRITE,
		0,
		uintptr(size),
		uintptr(unsafe.Pointer(namePtr)),
	)
	if h == 0 {
		return fmt.Errorf("CreateFileMapping: %w", err)
	}
	p.handle = h

	addr, _, err := procMapViewOfFile.Call(h, _FILE_MAP_ALL_ACCESS, 0, 0, uintptr(size))
	if addr == 0 {
		procCloseHandle.Call(h)
		return fmt.Errorf("MapViewOfFile: %w", err)
	}
	p.addr = addr
	p.buf = unsafe.Slice((*byte)(unsafe.Pointer(addr)), size)
	return nil
}

func (p *platformShm) open(name string, size int) error {
	p.size = size
	namePtr := utf16Ptr(name)

	h, _, err := procOpenFileMapping.Call(
		_FILE_MAP_ALL_ACCESS,
		0, // no inherit
		uintptr(unsafe.Pointer(namePtr)),
	)
	if h == 0 {
		return fmt.Errorf("OpenFileMapping: %w", err)
	}
	p.handle = h

	addr, _, err := procMapViewOfFile.Call(h, _FILE_MAP_ALL_ACCESS, 0, 0, uintptr(size))
	if addr == 0 {
		procCloseHandle.Call(h)
		return fmt.Errorf("MapViewOfFile: %w", err)
	}
	p.addr = addr
	p.buf = unsafe.Slice((*byte)(unsafe.Pointer(addr)), size)
	return nil
}

func (p *platformShm) data() []byte { return p.buf }

func (p *platformShm) close(_ bool) error {
	if p.addr != 0 {
		procUnmapViewOfFile.Call(p.addr)
		p.addr = 0
	}
	if p.handle != 0 {
		procCloseHandle.Call(p.handle)
		p.handle = 0
	}
	p.buf = nil
	return nil
}
