//go:build linux || darwin

package shm

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

type platformShm struct {
	file *os.File
	buf  []byte
	path string
}

func shmDir() string {
	if runtime.GOOS == "linux" {
		// tmpfs – backed by RAM, fastest option
		if info, err := os.Stat("/dev/shm"); err == nil && info.IsDir() {
			return "/dev/shm"
		}
	}
	return os.TempDir()
}

func (p *platformShm) create(name string, size int) error {
	p.path = filepath.Join(shmDir(), "vp_"+name)

	f, err := os.OpenFile(p.path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create file %s: %w", p.path, err)
	}
	p.file = f

	if err := f.Truncate(int64(size)); err != nil {
		f.Close()
		os.Remove(p.path)
		return fmt.Errorf("truncate: %w", err)
	}

	buf, err := syscall.Mmap(int(f.Fd()), 0, size,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		os.Remove(p.path)
		return fmt.Errorf("mmap: %w", err)
	}
	p.buf = buf
	return nil
}

func (p *platformShm) open(name string, size int) error {
	p.path = filepath.Join(shmDir(), "vp_"+name)

	f, err := os.OpenFile(p.path, os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open file %s: %w", p.path, err)
	}
	p.file = f

	buf, err := syscall.Mmap(int(f.Fd()), 0, size,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return fmt.Errorf("mmap: %w", err)
	}
	p.buf = buf
	return nil
}

func (p *platformShm) data() []byte { return p.buf }

func (p *platformShm) close(owner bool) error {
	if p.buf != nil {
		_ = syscall.Munmap(p.buf)
		p.buf = nil
	}
	if p.file != nil {
		p.file.Close()
		p.file = nil
	}
	if owner && p.path != "" {
		os.Remove(p.path)
	}
	return nil
}
