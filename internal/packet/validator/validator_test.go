package validator

import (
	"testing"
	"tunnox-core/internal/packet"

	"github.com/stretchr/testify/assert"
)

func TestNewDefaultPacketValidator(t *testing.T) {
	validator := NewDefaultPacketValidator()
	assert.NotNil(t, validator)
}

func TestDefaultPacketValidator_ValidateTransferPacket(t *testing.T) {
	validator := NewDefaultPacketValidator()

	tests := []struct {
		name        string
		transferPkt *packet.TransferPacket
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil transfer packet",
			transferPkt: nil,
			expectError: true,
			errorMsg:    "transfer packet is nil",
		},
		{
			name: "valid transfer packet with command",
			transferPkt: &packet.TransferPacket{
				PacketType: packet.JsonCommand,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.Connect,
					CommandId:   "cmd-001",
					Token:       "token",
					SenderId:    "sender",
					ReceiverId:  "receiver",
				},
			},
			expectError: false,
		},
		{
			name: "valid transfer packet without command",
			transferPkt: &packet.TransferPacket{
				PacketType:    packet.Heartbeat,
				CommandPacket: nil,
			},
			expectError: false,
		},
		{
			name: "invalid packet type",
			transferPkt: &packet.TransferPacket{
				PacketType: packet.Type(99),
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.Connect,
					CommandId:   "cmd",
					SenderId:    "s",
					ReceiverId:  "r",
				},
			},
			expectError: true,
			errorMsg:    "invalid packet type",
		},
		{
			name: "invalid command packet - nil command type",
			transferPkt: &packet.TransferPacket{
				PacketType: packet.JsonCommand,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.CommandType(0),
					CommandId:   "",
					SenderId:    "",
					ReceiverId:  "",
				},
			},
			expectError: true,
		},
		{
			name: "valid compressed packet",
			transferPkt: &packet.TransferPacket{
				PacketType: packet.Compressed,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.TcpMapCreate,
					CommandId:   "tcp-001",
					SenderId:    "client",
					ReceiverId:  "server",
				},
			},
			expectError: false,
		},
		{
			name: "valid encrypted packet",
			transferPkt: &packet.TransferPacket{
				PacketType: packet.Encrypted,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.HttpMapCreate,
					CommandId:   "http-001",
					SenderId:    "client",
					ReceiverId:  "server",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateTransferPacket(tt.transferPkt)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultPacketValidator_ValidateCommandPacket(t *testing.T) {
	validator := NewDefaultPacketValidator()

	tests := []struct {
		name        string
		commandPkt  *packet.CommandPacket
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil command packet",
			commandPkt:  nil,
			expectError: true,
			errorMsg:    "command packet is nil",
		},
		{
			name: "valid command packet",
			commandPkt: &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "cmd-001",
				Token:       "token",
				SenderId:    "sender-1",
				ReceiverId:  "receiver-1",
				CommandBody: `{"data":"value"}`,
			},
			expectError: false,
		},
		{
			name: "empty command ID",
			commandPkt: &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "",
				SenderId:    "sender",
				ReceiverId:  "receiver",
			},
			expectError: true,
			errorMsg:    "command ID is empty",
		},
		{
			name: "empty sender ID",
			commandPkt: &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "cmd-001",
				SenderId:    "",
				ReceiverId:  "receiver",
			},
			expectError: true,
			errorMsg:    "sender ID is empty",
		},
		{
			name: "empty receiver ID",
			commandPkt: &packet.CommandPacket{
				CommandType: packet.Connect,
				CommandId:   "cmd-001",
				SenderId:    "sender",
				ReceiverId:  "",
			},
			expectError: true,
			errorMsg:    "receiver ID is empty",
		},
		{
			name: "invalid command type",
			commandPkt: &packet.CommandPacket{
				CommandType: packet.CommandType(255),
				CommandId:   "cmd-001",
				SenderId:    "sender",
				ReceiverId:  "receiver",
			},
			expectError: true,
			errorMsg:    "invalid command type",
		},
		{
			name: "valid command with empty token",
			commandPkt: &packet.CommandPacket{
				CommandType: packet.HeartbeatCmd,
				CommandId:   "hb-001",
				Token:       "",
				SenderId:    "client",
				ReceiverId:  "server",
			},
			expectError: false,
		},
		{
			name: "valid command with empty body",
			commandPkt: &packet.CommandPacket{
				CommandType: packet.Disconnect,
				CommandId:   "disc-001",
				Token:       "token",
				SenderId:    "client",
				ReceiverId:  "server",
				CommandBody: "",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCommandPacket(tt.commandPkt)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultPacketValidator_ValidatePacketType(t *testing.T) {
	validator := NewDefaultPacketValidator()

	tests := []struct {
		name        string
		packetType  packet.Type
		expectError bool
	}{
		{
			name:        "valid JsonCommand",
			packetType:  packet.JsonCommand,
			expectError: false,
		},
		{
			name:        "valid Compressed",
			packetType:  packet.Compressed,
			expectError: false,
		},
		{
			name:        "valid Encrypted",
			packetType:  packet.Encrypted,
			expectError: false,
		},
		{
			name:        "valid Heartbeat",
			packetType:  packet.Heartbeat,
			expectError: false,
		},
		{
			name:        "invalid packet type - 0",
			packetType:  packet.Type(0),
			expectError: true,
		},
		{
			name:        "invalid packet type - 99",
			packetType:  packet.Type(99),
			expectError: true,
		},
		{
			name:        "invalid packet type - 255",
			packetType:  packet.Type(255),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidatePacketType(tt.packetType)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid packet type")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultPacketValidator_ValidateCommandType(t *testing.T) {
	validator := NewDefaultPacketValidator()

	validCommands := []packet.CommandType{
		packet.Connect,
		packet.Disconnect,
		packet.Reconnect,
		packet.HeartbeatCmd,
		packet.TcpMapCreate,
		packet.TcpMapDelete,
		packet.TcpMapUpdate,
		packet.TcpMapList,
		packet.TcpMapStatus,
		packet.HttpMapCreate,
		packet.HttpMapDelete,
		packet.HttpMapUpdate,
		packet.HttpMapList,
		packet.HttpMapStatus,
		packet.SocksMapCreate,
		packet.SocksMapDelete,
		packet.SocksMapUpdate,
		packet.SocksMapList,
		packet.SocksMapStatus,
		packet.DataTransferStart,
		packet.DataTransferStop,
		packet.DataTransferStatus,
		packet.ProxyForward,
		packet.DataTransferOut,
		packet.ConfigGet,
		packet.ConfigSet,
		packet.StatsGet,
		packet.LogGet,
		packet.HealthCheck,
		packet.RpcInvoke,
		packet.RpcRegister,
		packet.RpcUnregister,
		packet.RpcList,
	}

	t.Run("all valid commands", func(t *testing.T) {
		for _, cmd := range validCommands {
			err := validator.ValidateCommandType(cmd)
			assert.NoError(t, err, "command type %v should be valid", cmd)
		}
	})

	invalidCommands := []packet.CommandType{
		packet.CommandType(0),
		packet.CommandType(200),
		packet.CommandType(255),
	}

	t.Run("invalid commands", func(t *testing.T) {
		for _, cmd := range invalidCommands {
			err := validator.ValidateCommandType(cmd)
			assert.Error(t, err, "command type %v should be invalid", cmd)
			assert.Contains(t, err.Error(), "invalid command type")
		}
	})
}

func TestDefaultPacketValidator_CompleteValidation(t *testing.T) {
	validator := NewDefaultPacketValidator()

	// Test complete validation flow
	tests := []struct {
		name        string
		transferPkt *packet.TransferPacket
		expectError bool
	}{
		{
			name: "fully valid transfer packet",
			transferPkt: &packet.TransferPacket{
				PacketType: packet.JsonCommand,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.TcpMapCreate,
					CommandId:   "tcp-map-001",
					Token:       "secure-token",
					SenderId:    "client-123",
					ReceiverId:  "server-456",
					CommandBody: `{"local_port":8080,"remote_port":80}`,
				},
			},
			expectError: false,
		},
		{
			name: "invalid nested - bad command type",
			transferPkt: &packet.TransferPacket{
				PacketType: packet.JsonCommand,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.CommandType(250),
					CommandId:   "cmd",
					SenderId:    "s",
					ReceiverId:  "r",
				},
			},
			expectError: true,
		},
		{
			name: "invalid nested - empty command ID",
			transferPkt: &packet.TransferPacket{
				PacketType: packet.JsonCommand,
				CommandPacket: &packet.CommandPacket{
					CommandType: packet.Connect,
					CommandId:   "",
					SenderId:    "s",
					ReceiverId:  "r",
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateTransferPacket(tt.transferPkt)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultPacketValidator_EdgeCases(t *testing.T) {
	validator := NewDefaultPacketValidator()

	t.Run("transfer packet with nil command but JsonCommand type", func(t *testing.T) {
		transferPkt := &packet.TransferPacket{
			PacketType:    packet.JsonCommand,
			CommandPacket: nil,
		}
		// Should pass - command packet is optional
		err := validator.ValidateTransferPacket(transferPkt)
		assert.NoError(t, err)
	})

	t.Run("heartbeat packet without command", func(t *testing.T) {
		transferPkt := &packet.TransferPacket{
			PacketType:    packet.Heartbeat,
			CommandPacket: nil,
		}
		err := validator.ValidateTransferPacket(transferPkt)
		assert.NoError(t, err)
	})

	t.Run("command packet with very long IDs", func(t *testing.T) {
		longID := ""
		for i := 0; i < 1000; i++ {
			longID += "x"
		}
		commandPkt := &packet.CommandPacket{
			CommandType: packet.Connect,
			CommandId:   longID,
			SenderId:    longID,
			ReceiverId:  longID,
		}
		err := validator.ValidateCommandPacket(commandPkt)
		assert.NoError(t, err)
	})

	t.Run("command packet with special characters", func(t *testing.T) {
		commandPkt := &packet.CommandPacket{
			CommandType: packet.TcpMapCreate,
			CommandId:   "cmd-!@#$%^&*()",
			Token:       "token-🔒",
			SenderId:    "sender-中文",
			ReceiverId:  "receiver-Ελληνικά",
			CommandBody: `{"unicode":"✓"}`,
		}
		err := validator.ValidateCommandPacket(commandPkt)
		assert.NoError(t, err)
	})
}
