package sidecar

import "errors"

var (
	ErrProtocolPayloadTooLarge = errors.New("protocol: payload too large")

	// ErrProtocolMarshal
	ErrProtocolMarshal = errors.New("protocol: marshal")

	ErrProtocolWriteHeader = errors.New("protocol: write header")

	ErrProtocolWriteBody = errors.New("protocol: write body")

	ErrProtocolReadHeader = errors.New("protocol: read header")

	ErrProtocolReadBody = errors.New("protocol: read body")
)
