package relay

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	FrameMagic   byte = 0x77
	FrameVersion byte = 0x01

	TypeHandshake byte = 0x01
	TypeData      byte = 0x02
	TypePing      byte = 0x03
	TypePong      byte = 0x04
	TypeClose     byte = 0x05

	MaxPayloadSize = 4 * 1024 * 1024 // 4 MB max frame size
)

type Frame struct {
	Type     byte
	SourceID string
	TargetID string
	Payload  []byte
}

func WriteFrame(w io.Writer, f *Frame) error {
	header := make([]byte, 1+1+1+32+32+4) // magic(1) + ver(1) + type(1) + src(32) + dst(32) + len(4)
	header[0] = FrameMagic
	header[1] = FrameVersion
	header[2] = f.Type

	copy(header[3:35], f.SourceID)
	copy(header[35:67], f.TargetID)

	payloadLen := uint32(len(f.Payload))
	if payloadLen > MaxPayloadSize {
		return fmt.Errorf("payload size %d exceeds max frame size %d", payloadLen, MaxPayloadSize)
	}

	binary.BigEndian.PutUint32(header[67:71], payloadLen)

	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("failed to write frame header: %w", err)
	}

	if payloadLen > 0 {
		if _, err := w.Write(f.Payload); err != nil {
			return fmt.Errorf("failed to write frame payload: %w", err)
		}
	}

	return nil
}

func ReadFrame(r io.Reader) (*Frame, error) {
	header := make([]byte, 71)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	if header[0] != FrameMagic {
		return nil, fmt.Errorf("invalid frame magic byte: %x", header[0])
	}
	if header[1] != FrameVersion {
		return nil, fmt.Errorf("unsupported frame version: %x", header[1])
	}

	frameType := header[2]
	sourceID := strings.TrimRight(string(header[3:35]), "\x00")
	targetID := strings.TrimRight(string(header[35:67]), "\x00")
	payloadLen := binary.BigEndian.Uint32(header[67:71])

	if payloadLen > MaxPayloadSize {
		return nil, fmt.Errorf("payload size %d exceeds limit", payloadLen)
	}

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf("failed to read payload bytes: %w", err)
		}
	}

	return &Frame{
		Type:     frameType,
		SourceID: sourceID,
		TargetID: targetID,
		Payload:  payload,
	}, nil
}

func ValidateDeviceID(id string) error {
	if len(id) == 0 {
		return errors.New("device ID cannot be empty")
	}
	return nil
}
