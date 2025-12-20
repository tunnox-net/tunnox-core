package packet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// TransferPacket V2 扩展测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestTransferPacket_V1Compatibility(t *testing.T) {
	// V1格式：Flags = 0
	pkt := &TransferPacket{
		PacketType: TunnelData,
		TunnelID:   "tunnel-123",
		Payload:    []byte("hello"),
		Flags:      FlagNone,
	}

	assert.False(t, pkt.IsV2(), "V1 packet should not be V2")
	assert.Equal(t, FlagNone, pkt.Flags)
}

func TestTransferPacket_V2Format(t *testing.T) {
	// V2格式：Flags != 0
	pkt := &TransferPacket{
		PacketType: TunnelData,
		TunnelID:   "tunnel-123",
		Payload:    []byte("hello"),
		SeqNum:     100,
		AckNum:     50,
		Flags:      FlagACK,
	}

	assert.True(t, pkt.IsV2(), "V2 packet should be V2")
	assert.Equal(t, uint64(100), pkt.SeqNum)
	assert.Equal(t, uint64(50), pkt.AckNum)
}

func TestPacketFlags_HasFlag(t *testing.T) {
	pkt := &TransferPacket{
		Flags: FlagSYN | FlagACK,
	}

	assert.True(t, pkt.HasFlag(FlagSYN), "Should have SYN flag")
	assert.True(t, pkt.HasFlag(FlagACK), "Should have ACK flag")
	assert.False(t, pkt.HasFlag(FlagFIN), "Should not have FIN flag")
	assert.False(t, pkt.HasFlag(FlagRST), "Should not have RST flag")
}

func TestPacketFlags_SetFlag(t *testing.T) {
	pkt := &TransferPacket{
		Flags: FlagNone,
	}

	pkt.SetFlag(FlagSYN)
	assert.True(t, pkt.HasFlag(FlagSYN), "Should have SYN flag after setting")
	assert.True(t, pkt.IsV2(), "Should be V2 after setting flag")

	pkt.SetFlag(FlagACK)
	assert.True(t, pkt.HasFlag(FlagSYN), "Should still have SYN flag")
	assert.True(t, pkt.HasFlag(FlagACK), "Should have ACK flag")
}

func TestPacketFlags_ClearFlag(t *testing.T) {
	pkt := &TransferPacket{
		Flags: FlagSYN | FlagACK | FlagFIN,
	}

	pkt.ClearFlag(FlagACK)
	assert.True(t, pkt.HasFlag(FlagSYN), "Should still have SYN flag")
	assert.False(t, pkt.HasFlag(FlagACK), "Should not have ACK flag after clearing")
	assert.True(t, pkt.HasFlag(FlagFIN), "Should still have FIN flag")
}

func TestPacketFlags_Multiple(t *testing.T) {
	pkt := &TransferPacket{}

	// 设置多个标志
	pkt.SetFlag(FlagSYN)
	pkt.SetFlag(FlagACK)
	pkt.SetFlag(FlagMigrate)

	assert.True(t, pkt.HasFlag(FlagSYN))
	assert.True(t, pkt.HasFlag(FlagACK))
	assert.True(t, pkt.HasFlag(FlagMigrate))
	assert.False(t, pkt.HasFlag(FlagFIN))

	// 清除一个标志
	pkt.ClearFlag(FlagACK)
	assert.True(t, pkt.HasFlag(FlagSYN))
	assert.False(t, pkt.HasFlag(FlagACK))
	assert.True(t, pkt.HasFlag(FlagMigrate))
}

func TestPacketFlags_AllFlags(t *testing.T) {
	testCases := []struct {
		name string
		flag PacketFlags
	}{
		{"FlagNone", FlagNone},
		{"FlagSYN", FlagSYN},
		{"FlagFIN", FlagFIN},
		{"FlagACK", FlagACK},
		{"FlagRST", FlagRST},
		{"FlagMigrate", FlagMigrate},
		{"FlagBuffer", FlagBuffer},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pkt := &TransferPacket{
				Flags: tc.flag,
			}

			if tc.flag == FlagNone {
				assert.False(t, pkt.IsV2())
			} else {
				assert.True(t, pkt.IsV2())
				assert.True(t, pkt.HasFlag(tc.flag))
			}
		})
	}
}

func TestTransferPacket_SeqNumSequence(t *testing.T) {
	packets := make([]*TransferPacket, 5)

	for i := range packets {
		packets[i] = &TransferPacket{
			PacketType: TunnelData,
			TunnelID:   "tunnel-123",
			SeqNum:     uint64(i + 1),
			AckNum:     uint64(i),
			Flags:      FlagACK,
		}
	}

	// 验证序列号递增
	for i := 0; i < len(packets)-1; i++ {
		assert.Equal(t, packets[i].SeqNum+1, packets[i+1].SeqNum,
			"Sequence numbers should increment")
		assert.Equal(t, packets[i].AckNum+1, packets[i+1].AckNum,
			"Ack numbers should increment")
	}
}

func TestTransferPacket_ConnectionHandshake(t *testing.T) {
	// SYN packet (client -> server)
	syn := &TransferPacket{
		PacketType: TunnelOpen,
		TunnelID:   "tunnel-123",
		SeqNum:     1,
		Flags:      FlagSYN,
	}
	assert.True(t, syn.HasFlag(FlagSYN))
	assert.False(t, syn.HasFlag(FlagACK))

	// SYN-ACK packet (server -> client)
	synAck := &TransferPacket{
		PacketType: TunnelOpenAck,
		TunnelID:   "tunnel-123",
		SeqNum:     1,
		AckNum:     2, // Ack for client's SYN
		Flags:      FlagSYN | FlagACK,
	}
	assert.True(t, synAck.HasFlag(FlagSYN))
	assert.True(t, synAck.HasFlag(FlagACK))

	// ACK packet (client -> server)
	ack := &TransferPacket{
		PacketType: TunnelData,
		TunnelID:   "tunnel-123",
		SeqNum:     2,
		AckNum:     2, // Ack for server's SYN-ACK
		Flags:      FlagACK,
	}
	assert.False(t, ack.HasFlag(FlagSYN))
	assert.True(t, ack.HasFlag(FlagACK))
}

func TestTransferPacket_ConnectionClose(t *testing.T) {
	// FIN packet
	fin := &TransferPacket{
		PacketType: TunnelClose,
		TunnelID:   "tunnel-123",
		SeqNum:     100,
		AckNum:     50,
		Flags:      FlagFIN | FlagACK,
	}
	assert.True(t, fin.HasFlag(FlagFIN))
	assert.True(t, fin.HasFlag(FlagACK))

	// FIN-ACK packet
	finAck := &TransferPacket{
		PacketType: TunnelClose,
		TunnelID:   "tunnel-123",
		SeqNum:     50,
		AckNum:     101, // Ack for FIN
		Flags:      FlagFIN | FlagACK,
	}
	assert.True(t, finAck.HasFlag(FlagFIN))
	assert.True(t, finAck.HasFlag(FlagACK))
}

func TestTransferPacket_MigrationFlag(t *testing.T) {
	pkt := &TransferPacket{
		PacketType: TunnelData,
		TunnelID:   "tunnel-123",
		SeqNum:     100,
		Flags:      FlagMigrate | FlagBuffer,
	}

	assert.True(t, pkt.HasFlag(FlagMigrate), "Should have Migrate flag")
	assert.True(t, pkt.HasFlag(FlagBuffer), "Should have Buffer flag")
	assert.True(t, pkt.IsV2(), "Should be V2 format")
}
