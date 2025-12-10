package reliable

import (
	"encoding/binary"
	"fmt"
)

// Protocol constants
const (
	// MaxUDPPacketSize Maximum UDP packet size (1500 MTU - 20 IP - 8 UDP = 1472 bytes)
	MaxUDPPacketSize = 1472

	// MaxPayloadSize Maximum payload size after header
	MaxPayloadSize = MaxUDPPacketSize - HeaderSize

	// HeaderSize Size of packet header
	HeaderSize = 28

	// Version Protocol version
	Version = 1

	// MaxRetries Maximum number of retransmissions
	MaxRetries = 8

	// InitialRTO Initial retransmission timeout (1 second)
	InitialRTO = 1000 // milliseconds

	// MinRTO Minimum RTO value
	MinRTO = 200 // milliseconds

	// MaxRTO Maximum RTO value
	MaxRTO = 60000 // milliseconds

	// MaxWindowSize Maximum window size (packets)
	MaxWindowSize = 256

	// InitialWindowSize Initial congestion window size
	InitialWindowSize = 10

	// RTT Alpha for exponential weighted moving average (0.125)
	RTTAlpha = 0.125

	// RTT Beta for deviation calculation (0.25)
	RTTBeta = 0.25

	// SessionIdleTimeout Session idle timeout (15 minutes)
	// If no data is sent or received for this duration, the session will be closed
	SessionIdleTimeout = 15 * 60 * 1000 // milliseconds

	// KeepAliveInterval KeepAlive packet interval (30 seconds)
	KeepAliveInterval = 30 * 1000 // milliseconds
)

// PacketType defines the type of packet
type PacketType uint8

const (
	// PacketTypeSYN Handshake SYN packet
	PacketTypeSYN PacketType = 1

	// PacketTypeSYNACK Handshake SYN-ACK packet
	PacketTypeSYNACK PacketType = 2

	// PacketTypeACK Handshake ACK packet (connection established)
	PacketTypeACK PacketType = 3

	// PacketTypeData Data packet
	PacketTypeData PacketType = 4

	// PacketTypeDataACK Data acknowledgment packet
	PacketTypeDataACK PacketType = 5

	// PacketTypeFIN Connection close packet
	PacketTypeFIN PacketType = 6

	// PacketTypeFINACK Connection close acknowledgment
	PacketTypeFINACK PacketType = 7

	// PacketTypeRST Connection reset packet
	PacketTypeRST PacketType = 8
)

// String returns the string representation of PacketType
func (pt PacketType) String() string {
	switch pt {
	case PacketTypeSYN:
		return "SYN"
	case PacketTypeSYNACK:
		return "SYN-ACK"
	case PacketTypeACK:
		return "ACK"
	case PacketTypeData:
		return "DATA"
	case PacketTypeDataACK:
		return "DATA-ACK"
	case PacketTypeFIN:
		return "FIN"
	case PacketTypeFINACK:
		return "FIN-ACK"
	case PacketTypeRST:
		return "RST"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", pt)
	}
}

// PacketFlags defines flags for packet options
type PacketFlags uint8

const (
	// FlagNone No flags
	FlagNone PacketFlags = 0

	// FlagRetransmission Indicates this is a retransmitted packet
	FlagRetransmission PacketFlags = 1 << 0

	// FlagECN Explicit Congestion Notification
	FlagECN PacketFlags = 1 << 1
)

// PacketHeader UDP reliable protocol packet header (28 bytes)
type PacketHeader struct {
	// Version Protocol version (1 byte)
	Version uint8

	// Type Packet type (1 byte)
	Type PacketType

	// Flags Packet flags (1 byte)
	Flags PacketFlags

	// Reserved Reserved for future use (1 byte)
	Reserved uint8

	// SessionID Session identifier (4 bytes)
	SessionID uint32

	// StreamID Stream identifier (4 bytes)
	StreamID uint32

	// SequenceNum Packet sequence number (4 bytes)
	SequenceNum uint32

	// AckNum Acknowledgment number (4 bytes)
	AckNum uint32

	// WindowSize Receiver window size (2 bytes)
	WindowSize uint16

	// PayloadLen Payload length (2 bytes)
	PayloadLen uint16

	// Timestamp Timestamp in milliseconds (4 bytes)
	Timestamp uint32
}

// EncodeHeader encodes packet header to bytes
func EncodeHeader(h *PacketHeader) []byte {
	buf := make([]byte, HeaderSize)

	buf[0] = h.Version
	buf[1] = uint8(h.Type)
	buf[2] = uint8(h.Flags)
	buf[3] = h.Reserved

	binary.BigEndian.PutUint32(buf[4:8], h.SessionID)
	binary.BigEndian.PutUint32(buf[8:12], h.StreamID)
	binary.BigEndian.PutUint32(buf[12:16], h.SequenceNum)
	binary.BigEndian.PutUint32(buf[16:20], h.AckNum)
	binary.BigEndian.PutUint16(buf[20:22], h.WindowSize)
	binary.BigEndian.PutUint16(buf[22:24], h.PayloadLen)
	binary.BigEndian.PutUint32(buf[24:28], h.Timestamp)

	return buf
}

// DecodeHeader decodes packet header from bytes
func DecodeHeader(data []byte) (*PacketHeader, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("packet too small: %d bytes (need %d)", len(data), HeaderSize)
	}

	h := &PacketHeader{
		Version:     data[0],
		Type:        PacketType(data[1]),
		Flags:       PacketFlags(data[2]),
		Reserved:    data[3],
		SessionID:   binary.BigEndian.Uint32(data[4:8]),
		StreamID:    binary.BigEndian.Uint32(data[8:12]),
		SequenceNum: binary.BigEndian.Uint32(data[12:16]),
		AckNum:      binary.BigEndian.Uint32(data[16:20]),
		WindowSize:  binary.BigEndian.Uint16(data[20:22]),
		PayloadLen:  binary.BigEndian.Uint16(data[22:24]),
		Timestamp:   binary.BigEndian.Uint32(data[24:28]),
	}

	if h.Version != Version {
		return nil, fmt.Errorf("unsupported protocol version: %d", h.Version)
	}

	if h.PayloadLen > MaxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d bytes (max %d)", h.PayloadLen, MaxPayloadSize)
	}

	return h, nil
}

// Packet represents a complete UDP reliable protocol packet
type Packet struct {
	Header  *PacketHeader
	Payload []byte
}

// EncodePacket encodes a complete packet to bytes
func EncodePacket(pkt *Packet) []byte {
	headerBytes := EncodeHeader(pkt.Header)
	if pkt.Payload == nil {
		return headerBytes
	}
	return append(headerBytes, pkt.Payload...)
}

// DecodePacket decodes a complete packet from bytes
func DecodePacket(data []byte) (*Packet, error) {
	header, err := DecodeHeader(data)
	if err != nil {
		return nil, err
	}

	pkt := &Packet{
		Header: header,
	}

	if header.PayloadLen > 0 {
		if len(data) < HeaderSize+int(header.PayloadLen) {
			return nil, fmt.Errorf("incomplete packet: expected %d bytes, got %d",
				HeaderSize+int(header.PayloadLen), len(data))
		}
		pkt.Payload = data[HeaderSize : HeaderSize+int(header.PayloadLen)]
	}

	return pkt, nil
}

// SessionKey uniquely identifies a session
type SessionKey struct {
	RemoteAddr string
	SessionID  uint32
}

// String returns string representation of session key
func (k SessionKey) String() string {
	return fmt.Sprintf("%s/%d", k.RemoteAddr, k.SessionID)
}
