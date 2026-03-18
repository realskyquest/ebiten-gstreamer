package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/realskyquest/ebiten-gstreamer/sidecar"
)

type MsgType uint32

const (
	// Commands (Client → Sidecar)
	CmdOpen      MsgType = 0x0001
	CmdPlay      MsgType = 0x0002
	CmdPause     MsgType = 0x0003
	CmdStop      MsgType = 0x0004
	CmdSeek      MsgType = 0x0005
	CmdSetVolume MsgType = 0x0006
	CmdSetRate   MsgType = 0x0007
	CmdSetLoop   MsgType = 0x0008
	CmdRewind    MsgType = 0x0009
	CmdShutdown  MsgType = 0x00FF

	// Events (Sidecar → Client)
	EvtReady        MsgType = 0x1001
	EvtStateChanged MsgType = 0x1002
	EvtPosition     MsgType = 0x1003
	EvtMediaInfo    MsgType = 0x1004
	EvtError        MsgType = 0x1005
	EvtEOS          MsgType = 0x1006
	EvtBuffering    MsgType = 0x1007
)

// Command payloads

type OpenPayload struct {
	URI             string  `json:"uri"`
	Width           int     `json:"width,omitempty"`
	Height          int     `json:"height,omitempty"`
	MaxBufferFrames uint    `json:"max_buffer_frames,omitempty"`
	Volume          float64 `json:"volume"`
	Loop            bool    `json:"loop"`
	Rate            float64 `json:"rate"`
}

type SeekPayload struct {
	PositionNs int64 `json:"position_ns"`
}

type VolumePayload struct {
	Volume float64 `json:"volume"`
}

type RatePayload struct {
	Rate float64 `json:"rate"`
}

type LoopPayload struct {
	Loop bool `json:"loop"`
}

// Event payloads

type StateChangedPayload struct {
	State   string `json:"state"` // "playing","paused","stopped","idle"
	Playing bool   `json:"playing"`
	EOS     bool   `json:"eos"`
}

type PositionPayload struct {
	PositionNs int64   `json:"position_ns"`
	DurationNs int64   `json:"duration_ns"`
	Volume     float64 `json:"volume"`
	Rate       float64 `json:"rate"`
	Playing    bool    `json:"playing"`
	EOS        bool    `json:"eos"`
	Loop       bool    `json:"loop"`
}

type MediaInfoPayload struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type ErrorPayload struct {
	Message string `json:"message"`
	Debug   string `json:"debug,omitempty"`
}

type BufferingPayload struct {
	Percent int `json:"percent"`
}

// Wire format
// [4 bytes: MsgType BE] [4 bytes: payload length BE] [N bytes: JSON]

const headerSize = 8
const maxPayloadSize = 4 * 1024 * 1024

type Message struct {
	Type    MsgType
	Payload json.RawMessage
}

// Decode decodes a Message into a struct.
func Decode[T any](msg *Message) (T, error) {
	var v T
	if len(msg.Payload) == 0 {
		return v, nil
	}
	err := json.Unmarshal(msg.Payload, &v)
	return v, err
}

type Conn struct {
	raw     net.Conn
	writeMu sync.Mutex
}

func NewConn(c net.Conn) *Conn {
	return &Conn{raw: c}
}

func (c *Conn) Close() error { return c.raw.Close() }

// Send sends a Message to the Sidecar.
func (c *Conn) Send(msgType MsgType, payload any) error {
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("%w: %w", sidecar.ErrProtocolMarshal, err)
		}
	}

	hdr := make([]byte, headerSize)
	binary.BigEndian.PutUint32(hdr[0:4], uint32(msgType))
	binary.BigEndian.PutUint32(hdr[4:8], uint32(len(body)))

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if _, err := c.raw.Write(hdr); err != nil {
		return fmt.Errorf("%w: %w", sidecar.ErrProtocolWriteHeader, err)
	}
	if len(body) > 0 {
		if _, err := c.raw.Write(body); err != nil {
			return fmt.Errorf("%w: %w", sidecar.ErrProtocolWriteBody, err)
		}
	}
	return nil
}

// Receive receives a Message from the Sidecar.
func (c *Conn) Receive() (*Message, error) {
	hdr := make([]byte, headerSize)
	if _, err := io.ReadFull(c.raw, hdr); err != nil {
		return nil, fmt.Errorf("%w: %w", sidecar.ErrProtocolReadHeader, err)
	}

	msgType := MsgType(binary.BigEndian.Uint32(hdr[0:4]))
	length := binary.BigEndian.Uint32(hdr[4:8])

	if length > uint32(maxPayloadSize) {
		return nil, fmt.Errorf("%w: %d", sidecar.ErrProtocolPayloadTooLarge, length)
	}

	var payload json.RawMessage
	if length > 0 {
		payload = make([]byte, length)
		if _, err := io.ReadFull(c.raw, payload); err != nil {
			return nil, fmt.Errorf("%w: %w", sidecar.ErrProtocolReadBody, err)
		}
	}

	return &Message{Type: msgType, Payload: payload}, nil
}
