//go:build js && wasm

package shm

type platformShm struct{}

func (p *platformShm) create(name string, size int) error {
	panic("shm: not implemented")
}

func (p *platformShm) open(name string, size int) error {
	panic("shm: not implemented")
}

func (p *platformShm) data() []byte {
	panic("shm: not implemented")
}

func (p *platformShm) close(owner bool) error {
	panic("shm: not implemented")
}
