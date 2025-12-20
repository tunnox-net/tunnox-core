package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

func TestPacketRouter_RegisterHandler(t *testing.T) {
	router := NewPacketRouter(&PacketRouterConfig{
		Logger: corelog.NewNopLogger(),
	})

	called := false
	handler := PacketHandlerFunc(func(connPacket *types.StreamPacket) error {
		called = true
		return nil
	})

	router.RegisterHandler(packet.Handshake, handler)

	// 路由数据包
	connPacket := &types.StreamPacket{
		ConnectionID: "conn-1",
		Packet: &packet.TransferPacket{
			PacketType: packet.Handshake,
		},
	}

	err := router.Route(connPacket)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestPacketRouter_UnregisterHandler(t *testing.T) {
	router := NewPacketRouter(&PacketRouterConfig{
		Logger: corelog.NewNopLogger(),
	})

	handler := PacketHandlerFunc(func(connPacket *types.StreamPacket) error {
		return nil
	})

	router.RegisterHandler(packet.Handshake, handler)
	router.UnregisterHandler(packet.Handshake)

	// 路由数据包应该失败
	connPacket := &types.StreamPacket{
		ConnectionID: "conn-1",
		Packet: &packet.TransferPacket{
			PacketType: packet.Handshake,
		},
	}

	err := router.Route(connPacket)
	assert.Error(t, err)
}

func TestPacketRouter_DefaultHandler(t *testing.T) {
	defaultCalled := false
	router := NewPacketRouter(&PacketRouterConfig{
		Logger: corelog.NewNopLogger(),
		DefaultHandler: PacketHandlerFunc(func(connPacket *types.StreamPacket) error {
			defaultCalled = true
			return nil
		}),
	})

	// 路由未注册的数据包类型
	connPacket := &types.StreamPacket{
		ConnectionID: "conn-1",
		Packet: &packet.TransferPacket{
			PacketType: packet.Handshake,
		},
	}

	err := router.Route(connPacket)
	require.NoError(t, err)
	assert.True(t, defaultCalled)
}

func TestPacketRouter_NilPacket(t *testing.T) {
	router := NewPacketRouter(&PacketRouterConfig{
		Logger: corelog.NewNopLogger(),
	})

	// 测试 nil 数据包
	err := router.Route(nil)
	assert.Error(t, err)

	// 测试 nil Packet 字段
	err = router.Route(&types.StreamPacket{ConnectionID: "conn-1"})
	assert.Error(t, err)
}

func TestPacketRouter_RouteByCategory(t *testing.T) {
	router := NewPacketRouter(&PacketRouterConfig{
		Logger: corelog.NewNopLogger(),
	})

	var handledType string

	commandHandler := PacketHandlerFunc(func(connPacket *types.StreamPacket) error {
		handledType = "command"
		return nil
	})

	handshakeHandler := PacketHandlerFunc(func(connPacket *types.StreamPacket) error {
		handledType = "handshake"
		return nil
	})

	heartbeatHandler := PacketHandlerFunc(func(connPacket *types.StreamPacket) error {
		handledType = "heartbeat"
		return nil
	})

	// 测试命令包
	handledType = ""
	err := router.RouteByCategory(
		&types.StreamPacket{
			ConnectionID: "conn-1",
			Packet: &packet.TransferPacket{
				PacketType: packet.JsonCommand,
			},
		},
		commandHandler, handshakeHandler, nil, heartbeatHandler,
	)
	require.NoError(t, err)
	assert.Equal(t, "command", handledType)

	// 测试握手包
	handledType = ""
	err = router.RouteByCategory(
		&types.StreamPacket{
			ConnectionID: "conn-1",
			Packet: &packet.TransferPacket{
				PacketType: packet.Handshake,
			},
		},
		commandHandler, handshakeHandler, nil, heartbeatHandler,
	)
	require.NoError(t, err)
	assert.Equal(t, "handshake", handledType)

	// 测试心跳包
	handledType = ""
	err = router.RouteByCategory(
		&types.StreamPacket{
			ConnectionID: "conn-1",
			Packet: &packet.TransferPacket{
				PacketType: packet.Heartbeat,
			},
		},
		commandHandler, handshakeHandler, nil, heartbeatHandler,
	)
	require.NoError(t, err)
	assert.Equal(t, "heartbeat", handledType)
}
