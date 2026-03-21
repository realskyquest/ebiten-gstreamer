package protocol

import (
	"encoding/binary"
	"net"
	"testing"
)

// TestSendReceive does a round-trip test.
func TestSendReceiveRoundtrip(t *testing.T) {
	client, server := net.Pipe()
	c, s := NewConn(client), NewConn(server)

	payload := &OpenPayload{URI: "file:///test.mp4", Volume: 0.8}
	go c.Send(CmdOpen, payload)

	msg, err := s.Receive()
	if err != nil || msg.Type != CmdOpen {
		t.Fatalf("unexpected: err=%v, type=%v", err, msg.Type)
	}
	got, _ := Decode[OpenPayload](msg)
	if got.URI != payload.URI {
		t.Errorf("URI mismatch: got %q", got.URI)
	}
}

// TestPayloadTooLarge does a malformed header test.
func TestPayloadTooLarge(t *testing.T) {
	client, server := net.Pipe()
	c, s := NewConn(client), NewConn(server)

	// Manually write an oversized header
	hdr := make([]byte, 8)
	binary.BigEndian.PutUint32(hdr[0:4], uint32(CmdPlay))
	binary.BigEndian.PutUint32(hdr[4:8], maxPayloadSize+1)
	go client.Write(hdr)

	_, err := s.Receive()
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
	_ = c
}
