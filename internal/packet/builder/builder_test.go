package builder

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

func TestNewDefaultPacketBuilder(t *testing.T) {
	builder := NewDefaultPacketBuilder()
	assert.NotNil(t, builder)
}

func TestDefaultPacketBuilder_BuildPacket(t *testing.T) {
	builder := NewDefaultPacketBuilder()

	tests := []struct {
		name          string
		transferPkt   *packet.TransferPacket
		expectedErr   bool
		validateFunc  func(t *testing.T, buf *bytes.Buffer)
	}{
		{
			name: "build packet with command packet",
			transferPkt: &packet.TransferPacket{
				PacketType: packet.JsonCommand,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.Connect,
					CommandId:   "cmd-123",
					Token:       "token-456",
					SenderId:    "sender-1",
					ReceiverId:  "receiver-1",
					CommandBody: `{"test":"data"}`,
				},
			},
			expectedErr: false,
			validateFunc: func(t *testing.T, buf *bytes.Buffer) {
				// Validate packet type
				packetType, err := buf.ReadByte()
				require.NoError(t, err)
				assert.Equal(t, byte(packet.JsonCommand), packetType)

				// Validate length
				lengthBytes := make([]byte, 4)
				_, err = io.ReadFull(buf, lengthBytes)
				require.NoError(t, err)
				length := binary.BigEndian.Uint32(lengthBytes)
				assert.Greater(t, length, uint32(0))

				// Validate command packet content
				data := make([]byte, length)
				_, err = io.ReadFull(buf, data)
				require.NoError(t, err)

				var cmdPkt packet.CommandPacket
				err = json.Unmarshal(data, &cmdPkt)
				require.NoError(t, err)
				assert.Equal(t, packet.Connect, cmdPkt.CommandType)
				assert.Equal(t, "cmd-123", cmdPkt.CommandId)
			},
		},
		{
			name: "build packet without command packet",
			transferPkt: &packet.TransferPacket{
				PacketType:    packet.Heartbeat,
				CommandPacket: nil,
			},
			expectedErr: false,
			validateFunc: func(t *testing.T, buf *bytes.Buffer) {
				// Validate packet type
				packetType, err := buf.ReadByte()
				require.NoError(t, err)
				assert.Equal(t, byte(packet.Heartbeat), packetType)

				// Validate length is 0
				lengthBytes := make([]byte, 4)
				_, err = io.ReadFull(buf, lengthBytes)
				require.NoError(t, err)
				length := binary.BigEndian.Uint32(lengthBytes)
				assert.Equal(t, uint32(0), length)
			},
		},
		{
			name: "build compressed packet",
			transferPkt: &packet.TransferPacket{
				PacketType: packet.Compressed,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.HeartbeatCmd,
					CommandId:   "hb-001",
					Token:       "token",
					SenderId:    "s1",
					ReceiverId:  "r1",
					CommandBody: "",
				},
			},
			expectedErr: false,
			validateFunc: func(t *testing.T, buf *bytes.Buffer) {
				packetType, err := buf.ReadByte()
				require.NoError(t, err)
				assert.Equal(t, byte(packet.Compressed), packetType)
			},
		},
		{
			name: "build encrypted packet",
			transferPkt: &packet.TransferPacket{
				PacketType: packet.Encrypted,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.TcpMapCreate,
					CommandId:   "tcp-001",
					Token:       "secure-token",
					SenderId:    "client-1",
					ReceiverId:  "server-1",
					CommandBody: `{"port":8080}`,
				},
			},
			expectedErr: false,
			validateFunc: func(t *testing.T, buf *bytes.Buffer) {
				packetType, err := buf.ReadByte()
				require.NoError(t, err)
				assert.Equal(t, byte(packet.Encrypted), packetType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := builder.BuildPacket(buf, tt.transferPkt)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, buf)
				}
			}
		})
	}
}

func TestDefaultPacketBuilder_BuildPacket_WriteErrors(t *testing.T) {
	builder := NewDefaultPacketBuilder()
	transferPkt := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.Connect,
			CommandId:   "cmd-123",
			Token:       "token",
			SenderId:    "sender",
			ReceiverId:  "receiver",
		},
	}

	// Test writer that fails on first write
	failWriter := &failingWriter{failAt: 0}
	err := builder.BuildPacket(failWriter, transferPkt)
	assert.Error(t, err)

	// Test writer that fails on second write (length)
	failWriter = &failingWriter{failAt: 1}
	err = builder.BuildPacket(failWriter, transferPkt)
	assert.Error(t, err)

	// Test writer that fails on third write (data)
	failWriter = &failingWriter{failAt: 2}
	err = builder.BuildPacket(failWriter, transferPkt)
	assert.Error(t, err)
}

func TestDefaultPacketBuilder_BuildCommandPacket(t *testing.T) {
	builder := NewDefaultPacketBuilder()

	tests := []struct {
		name        string
		commandType packet.CommandType
		commandID   string
		token       string
		senderID    string
		receiverID  string
		commandBody string
	}{
		{
			name:        "build connect command",
			commandType: packet.Connect,
			commandID:   "conn-001",
			token:       "auth-token",
			senderID:    "client-123",
			receiverID:  "server-456",
			commandBody: `{"version":"1.0"}`,
		},
		{
			name:        "build disconnect command",
			commandType: packet.Disconnect,
			commandID:   "disc-001",
			token:       "token",
			senderID:    "client",
			receiverID:  "server",
			commandBody: "",
		},
		{
			name:        "build tcp map create command",
			commandType: packet.TcpMapCreate,
			commandID:   "tcp-create-001",
			token:       "token",
			senderID:    "client",
			receiverID:  "server",
			commandBody: `{"local_port":8080,"remote_port":80}`,
		},
		{
			name:        "build empty command body",
			commandType: packet.HeartbeatCmd,
			commandID:   "hb-001",
			token:       "",
			senderID:    "s",
			receiverID:  "r",
			commandBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdPkt, err := builder.BuildCommandPacket(
				tt.commandType,
				tt.commandID,
				tt.token,
				tt.senderID,
				tt.receiverID,
				tt.commandBody,
			)

			assert.NoError(t, err)
			assert.NotNil(t, cmdPkt)
			assert.Equal(t, tt.commandType, cmdPkt.CommandType)
			assert.Equal(t, tt.commandID, cmdPkt.CommandId)
			assert.Equal(t, tt.token, cmdPkt.Token)
			assert.Equal(t, tt.senderID, cmdPkt.SenderId)
			assert.Equal(t, tt.receiverID, cmdPkt.ReceiverId)
			assert.Equal(t, tt.commandBody, cmdPkt.CommandBody)
		})
	}
}

func TestDefaultPacketBuilder_BuildTransferPacket(t *testing.T) {
	builder := NewDefaultPacketBuilder()

	tests := []struct {
		name          string
		packetType    packet.Type
		commandPacket *packet.CommandPacket
	}{
		{
			name:       "build transfer packet with command",
			packetType: packet.JsonCommand,
			commandPacket: &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "cmd-123",
				Token:       "token",
				SenderId:    "sender",
				ReceiverId:  "receiver",
			},
		},
		{
			name:          "build transfer packet without command",
			packetType:    packet.Heartbeat,
			commandPacket: nil,
		},
		{
			name:       "build compressed transfer packet",
			packetType: packet.Compressed,
			commandPacket: &packet.CommandPacket{
				CommandType: packet.DataTransferStart,
				CommandId:   "data-001",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transferPkt := builder.BuildTransferPacket(tt.packetType, tt.commandPacket)

			assert.NotNil(t, transferPkt)
			assert.Equal(t, tt.packetType, transferPkt.PacketType)
			assert.Equal(t, tt.commandPacket, transferPkt.CommandPacket)
		})
	}
}

func TestDefaultPacketBuilder_RoundTrip(t *testing.T) {
	builder := NewDefaultPacketBuilder()

	// Build command packet
	cmdPkt, err := builder.BuildCommandPacket(
		packet.TcpMapCreate,
		"map-001",
		"secure-token",
		"client-123",
		"server-456",
		`{"local_port":8080,"remote_port":80}`,
	)
	require.NoError(t, err)

	// Build transfer packet
	transferPkt := builder.BuildTransferPacket(packet.JsonCommand, cmdPkt)

	// Write to buffer
	buf := &bytes.Buffer{}
	err = builder.BuildPacket(buf, transferPkt)
	require.NoError(t, err)

	// Verify we can read back packet type
	packetType, err := buf.ReadByte()
	require.NoError(t, err)
	assert.Equal(t, byte(packet.JsonCommand), packetType)

	// Verify length
	lengthBytes := make([]byte, 4)
	_, err = io.ReadFull(buf, lengthBytes)
	require.NoError(t, err)
	length := binary.BigEndian.Uint32(lengthBytes)
	assert.Greater(t, length, uint32(0))

	// Verify data
	data := make([]byte, length)
	_, err = io.ReadFull(buf, data)
	require.NoError(t, err)

	var parsedCmd packet.CommandPacket
	err = json.Unmarshal(data, &parsedCmd)
	require.NoError(t, err)
	assert.Equal(t, cmdPkt.CommandType, parsedCmd.CommandType)
	assert.Equal(t, cmdPkt.CommandId, parsedCmd.CommandId)
	assert.Equal(t, cmdPkt.Token, parsedCmd.Token)
}

// failingWriter is a writer that fails after a certain number of writes
type failingWriter struct {
	failAt      int
	writeCount  int
}

func (w *failingWriter) Write(p []byte) (n int, err error) {
	if w.writeCount == w.failAt {
		return 0, errors.New("write failed")
	}
	w.writeCount++
	return len(p), nil
}
