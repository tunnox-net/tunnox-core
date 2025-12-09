package parser

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"tunnox-core/internal/packet"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultPacketParser(t *testing.T) {
	parser := NewDefaultPacketParser()
	assert.NotNil(t, parser)
}

func TestDefaultPacketParser_ParsePacket(t *testing.T) {
	parser := NewDefaultPacketParser()

	tests := []struct {
		name        string
		buildPacket func() *bytes.Buffer
		validate    func(t *testing.T, pkt *packet.TransferPacket)
		expectError bool
		errorMsg    string
	}{
		{
			name: "parse valid json command packet",
			buildPacket: func() *bytes.Buffer {
				buf := &bytes.Buffer{}
				// Write packet type
				buf.WriteByte(byte(packet.JsonCommand))

				// Build command packet
				cmdPkt := &packet.CommandPacket{
					CommandType: packet.Connect,
					CommandId:   "cmd-123",
					Token:       "token-456",
					SenderId:    "sender-1",
					ReceiverId:  "receiver-1",
					CommandBody: `{"test":"data"}`,
				}
				data, _ := json.Marshal(cmdPkt)

				// Write length
				lengthBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))
				buf.Write(lengthBytes)

				// Write data
				buf.Write(data)
				return buf
			},
			validate: func(t *testing.T, pkt *packet.TransferPacket) {
				assert.Equal(t, packet.JsonCommand, pkt.PacketType)
				assert.NotNil(t, pkt.CommandPacket)
				assert.Equal(t, packet.Connect, pkt.CommandPacket.CommandType)
				assert.Equal(t, "cmd-123", pkt.CommandPacket.CommandId)
				assert.Equal(t, "token-456", pkt.CommandPacket.Token)
				assert.Equal(t, "sender-1", pkt.CommandPacket.SenderId)
				assert.Equal(t, "receiver-1", pkt.CommandPacket.ReceiverId)
			},
			expectError: false,
		},
		{
			name: "parse heartbeat command packet",
			buildPacket: func() *bytes.Buffer {
				buf := &bytes.Buffer{}
				buf.WriteByte(byte(packet.JsonCommand))

				cmdPkt := &packet.CommandPacket{
					CommandType: packet.HeartbeatCmd,
					CommandId:   "hb-001",
					Token:       "",
					SenderId:    "client",
					ReceiverId:  "server",
					CommandBody: "",
				}
				data, _ := json.Marshal(cmdPkt)

				lengthBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))
				buf.Write(lengthBytes)
				buf.Write(data)
				return buf
			},
			validate: func(t *testing.T, pkt *packet.TransferPacket) {
				assert.Equal(t, packet.JsonCommand, pkt.PacketType)
				assert.NotNil(t, pkt.CommandPacket)
				assert.Equal(t, packet.HeartbeatCmd, pkt.CommandPacket.CommandType)
			},
			expectError: false,
		},
		{
			name: "parse compressed packet (with compression flag)",
			buildPacket: func() *bytes.Buffer {
				buf := &bytes.Buffer{}
				// Set compression flag (0x40) on JsonCommand
				buf.WriteByte(byte(packet.JsonCommand | 0x40))

				cmdPkt := &packet.CommandPacket{
					CommandType: packet.TcpMapCreate,
					CommandId:   "tcp-001",
					Token:       "token",
					SenderId:    "s",
					ReceiverId:  "r",
				}
				data, _ := json.Marshal(cmdPkt)

				lengthBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))
				buf.Write(lengthBytes)
				buf.Write(data)
				return buf
			},
			validate: func(t *testing.T, pkt *packet.TransferPacket) {
				// Parser should ignore compression flag and extract base type
				assert.NotNil(t, pkt.CommandPacket)
				assert.Equal(t, packet.TcpMapCreate, pkt.CommandPacket.CommandType)
			},
			expectError: false,
		},
		{
			name: "parse encrypted packet (with encryption flag)",
			buildPacket: func() *bytes.Buffer {
				buf := &bytes.Buffer{}
				// Set encryption flag (0x80) on JsonCommand
				buf.WriteByte(byte(packet.JsonCommand | 0x80))

				cmdPkt := &packet.CommandPacket{
					CommandType: packet.HttpMapCreate,
					CommandId:   "http-001",
					Token:       "secure",
					SenderId:    "client",
					ReceiverId:  "server",
				}
				data, _ := json.Marshal(cmdPkt)

				lengthBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))
				buf.Write(lengthBytes)
				buf.Write(data)
				return buf
			},
			validate: func(t *testing.T, pkt *packet.TransferPacket) {
				assert.NotNil(t, pkt.CommandPacket)
				assert.Equal(t, packet.HttpMapCreate, pkt.CommandPacket.CommandType)
			},
			expectError: false,
		},
		{
			name: "parse packet with empty data",
			buildPacket: func() *bytes.Buffer {
				buf := &bytes.Buffer{}
				buf.WriteByte(byte(packet.JsonCommand))

				// Zero length
				lengthBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(lengthBytes, 0)
				buf.Write(lengthBytes)
				return buf
			},
			expectError: true,
			errorMsg:    "unexpected end of JSON input",
		},
		{
			name: "parse unknown packet type",
			buildPacket: func() *bytes.Buffer {
				buf := &bytes.Buffer{}
				buf.WriteByte(0xFF) // Unknown type

				lengthBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(lengthBytes, 5)
				buf.Write(lengthBytes)
				buf.Write([]byte("dummy"))
				return buf
			},
			expectError: true,
			errorMsg:    "unknown packet type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := tt.buildPacket()
			pkt, err := parser.ParsePacket(buf)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, pkt)
				if tt.validate != nil {
					tt.validate(t, pkt)
				}
			}
		})
	}
}

func TestDefaultPacketParser_ParsePacket_ReadErrors(t *testing.T) {
	parser := NewDefaultPacketParser()

	tests := []struct {
		name        string
		reader      io.Reader
		errorMsg    string
	}{
		{
			name:     "error reading packet type",
			reader:   &errorReader{failAt: 0},
			errorMsg: "read error",
		},
		{
			name:     "error reading length",
			reader:   &partialReader{data: []byte{byte(packet.JsonCommand)}},
			errorMsg: "EOF",
		},
		{
			name: "error reading data",
			reader: &partialReader{
				data: append(
					[]byte{byte(packet.JsonCommand)},
					[]byte{0, 0, 0, 10}..., // length = 10
				),
			},
			errorMsg: "EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.ParsePacket(tt.reader)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

func TestDefaultPacketParser_ParseCommandPacket(t *testing.T) {
	parser := NewDefaultPacketParser()

	tests := []struct {
		name        string
		data        []byte
		validate    func(t *testing.T, pkt *packet.CommandPacket)
		expectError bool
	}{
		{
			name: "parse valid command packet",
			data: func() []byte {
				cmdPkt := &packet.CommandPacket{
					CommandType: packet.Connect,
					CommandId:   "cmd-001",
					Token:       "token",
					SenderId:    "sender",
					ReceiverId:  "receiver",
					CommandBody: `{"key":"value"}`,
				}
				data, _ := json.Marshal(cmdPkt)
				return data
			}(),
			validate: func(t *testing.T, pkt *packet.CommandPacket) {
				assert.Equal(t, packet.Connect, pkt.CommandType)
				assert.Equal(t, "cmd-001", pkt.CommandId)
				assert.Equal(t, "token", pkt.Token)
				assert.Equal(t, "sender", pkt.SenderId)
				assert.Equal(t, "receiver", pkt.ReceiverId)
				assert.Equal(t, `{"key":"value"}`, pkt.CommandBody)
			},
			expectError: false,
		},
		{
			name: "parse command packet with empty body",
			data: func() []byte {
				cmdPkt := &packet.CommandPacket{
					CommandType: packet.HeartbeatCmd,
					CommandId:   "hb",
					Token:       "",
					SenderId:    "s",
					ReceiverId:  "r",
					CommandBody: "",
				}
				data, _ := json.Marshal(cmdPkt)
				return data
			}(),
			validate: func(t *testing.T, pkt *packet.CommandPacket) {
				assert.Equal(t, packet.HeartbeatCmd, pkt.CommandType)
				assert.Empty(t, pkt.CommandBody)
			},
			expectError: false,
		},
		{
			name:        "parse invalid json",
			data:        []byte(`{invalid json}`),
			expectError: true,
		},
		{
			name:        "parse empty data",
			data:        []byte{},
			expectError: true,
		},
		{
			name:        "parse null json",
			data:        []byte("null"),
			expectError: false, // JSON null is valid, unmarshal will succeed but result will have zero values
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkt, err := parser.ParseCommandPacket(tt.data)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, pkt)
				if tt.validate != nil {
					tt.validate(t, pkt)
				}
			}
		})
	}
}

func TestDefaultPacketParser_RoundTrip(t *testing.T) {
	parser := NewDefaultPacketParser()

	// Build a packet
	buf := &bytes.Buffer{}
	buf.WriteByte(byte(packet.JsonCommand))

	cmdPkt := &packet.CommandPacket{
		CommandType: packet.TcpMapCreate,
		CommandId:   "tcp-map-001",
		Token:       "secure-token-xyz",
		SenderId:    "client-123",
		ReceiverId:  "server-456",
		CommandBody: `{"local_port":8080,"remote_port":80,"protocol":"tcp"}`,
	}
	data, err := json.Marshal(cmdPkt)
	require.NoError(t, err)

	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))
	buf.Write(lengthBytes)
	buf.Write(data)

	// Parse the packet
	parsedPkt, err := parser.ParsePacket(buf)
	require.NoError(t, err)
	require.NotNil(t, parsedPkt)
	require.NotNil(t, parsedPkt.CommandPacket)

	// Verify all fields match
	assert.Equal(t, packet.JsonCommand, parsedPkt.PacketType)
	assert.Equal(t, cmdPkt.CommandType, parsedPkt.CommandPacket.CommandType)
	assert.Equal(t, cmdPkt.CommandId, parsedPkt.CommandPacket.CommandId)
	assert.Equal(t, cmdPkt.Token, parsedPkt.CommandPacket.Token)
	assert.Equal(t, cmdPkt.SenderId, parsedPkt.CommandPacket.SenderId)
	assert.Equal(t, cmdPkt.ReceiverId, parsedPkt.CommandPacket.ReceiverId)
	assert.Equal(t, cmdPkt.CommandBody, parsedPkt.CommandPacket.CommandBody)
}

func TestDefaultPacketParser_MultiplePackets(t *testing.T) {
	parser := NewDefaultPacketParser()

	// Build buffer with multiple packets
	buf := &bytes.Buffer{}

	packets := []struct {
		commandType packet.CommandType
		commandID   string
	}{
		{packet.Connect, "cmd-1"},
		{packet.TcpMapCreate, "cmd-2"},
		{packet.HttpMapCreate, "cmd-3"},
	}

	for _, pkt := range packets {
		buf.WriteByte(byte(packet.JsonCommand))

		cmdPkt := &packet.CommandPacket{
			CommandType: pkt.commandType,
			CommandId:   pkt.commandID,
			Token:       "token",
			SenderId:    "s",
			ReceiverId:  "r",
		}
		data, _ := json.Marshal(cmdPkt)

		lengthBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))
		buf.Write(lengthBytes)
		buf.Write(data)
	}

	// Parse all packets
	for i, expected := range packets {
		parsedPkt, err := parser.ParsePacket(buf)
		require.NoError(t, err, "packet %d", i)
		require.NotNil(t, parsedPkt.CommandPacket, "packet %d", i)
		assert.Equal(t, expected.commandType, parsedPkt.CommandPacket.CommandType, "packet %d", i)
		assert.Equal(t, expected.commandID, parsedPkt.CommandPacket.CommandId, "packet %d", i)
	}
}

// errorReader always returns an error
type errorReader struct {
	failAt int
	readCount int
}

func (r *errorReader) Read(p []byte) (n int, err error) {
	if r.readCount >= r.failAt {
		return 0, errors.New("read error")
	}
	r.readCount++
	return 0, errors.New("read error")
}

// partialReader returns partial data then EOF
type partialReader struct {
	data []byte
	pos  int
}

func (r *partialReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= len(r.data) {
		return n, io.EOF
	}
	return n, nil
}
