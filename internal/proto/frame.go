package proto

import (
	"encoding/binary"
	"fmt"
	"io"
)

// MaxFrameSize is the largest frame (payload only) we will read or write.
const MaxFrameSize = 64 * 1024 // 64 KB

// WriteFrame writes data as a [4-byte big-endian length][data] frame to w.
func WriteFrame(w io.Writer, data []byte) error {
	if len(data) > MaxFrameSize {
		return fmt.Errorf("frame too large: %d bytes (max %d)", len(data), MaxFrameSize)
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(data)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("writing frame header: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("writing frame body: %w", err)
	}
	return nil
}

// ReadFrame reads one length-prefixed frame from r.
func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, fmt.Errorf("reading frame header: %w", err)
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > MaxFrameSize {
		return nil, fmt.Errorf("frame too large: %d bytes (max %d)", n, MaxFrameSize)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("reading frame body: %w", err)
	}
	return buf, nil
}
