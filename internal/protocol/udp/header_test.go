package udp

import (
	"testing"
)

func TestHeaderEncodeDecode(t *testing.T) {
	original := &TUTPHeader{
		Version:    TUTPVersion,
		Flags:      FlagACK | FlagSYN,
		SessionID:  12345,
		StreamID:   0,
		PacketSeq:  100,
		FragSeq:    0,
		FragCount:  1,
		AckSeq:     50,
		WindowSize: 64,
		Reserved:   0,
		Timestamp:  1234567890,
	}

	buf := make([]byte, HeaderLength())
	n, err := original.Encode(buf)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if n != HeaderLength() {
		t.Fatalf("Encode returned wrong length: expected %d, got %d", HeaderLength(), n)
	}

	decoded, n, err := DecodeHeader(buf)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if n != HeaderLength() {
		t.Fatalf("Decode returned wrong length: expected %d, got %d", HeaderLength(), n)
	}

	if decoded.Version != original.Version {
		t.Errorf("Version mismatch: expected %d, got %d", original.Version, decoded.Version)
	}
	if decoded.Flags != original.Flags {
		t.Errorf("Flags mismatch: expected %d, got %d", original.Flags, decoded.Flags)
	}
	if decoded.SessionID != original.SessionID {
		t.Errorf("SessionID mismatch: expected %d, got %d", original.SessionID, decoded.SessionID)
	}
	if decoded.PacketSeq != original.PacketSeq {
		t.Errorf("PacketSeq mismatch: expected %d, got %d", original.PacketSeq, decoded.PacketSeq)
	}
	if decoded.AckSeq != original.AckSeq {
		t.Errorf("AckSeq mismatch: expected %d, got %d", original.AckSeq, decoded.AckSeq)
	}
}

func TestDecodeHeaderInvalidVersion(t *testing.T) {
	buf := make([]byte, HeaderLength())
	header := &TUTPHeader{
		Version: 99, // 无效版本
	}
	header.Encode(buf)

	_, _, err := DecodeHeader(buf)
	if err == nil {
		t.Fatal("Expected error for invalid version, got nil")
	}
}

func TestDecodeHeaderInvalidFragCount(t *testing.T) {
	buf := make([]byte, HeaderLength())
	header := &TUTPHeader{
		Version:   TUTPVersion,
		FragCount: 0, // 无效分片数
	}
	header.Encode(buf)

	_, _, err := DecodeHeader(buf)
	if err == nil {
		t.Fatal("Expected error for invalid frag count, got nil")
	}
}

