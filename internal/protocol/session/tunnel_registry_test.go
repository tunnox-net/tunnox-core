package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corelog "tunnox-core/internal/core/log"
)

func TestTunnelRegistry_Register(t *testing.T) {
	registry := NewTunnelRegistry(&TunnelRegistryConfig{
		Logger: corelog.NewNopLogger(),
	})

	conn := &TunnelConnection{
		ConnID:   "conn-1",
		TunnelID: "tunnel-1",
	}

	err := registry.Register(conn)
	require.NoError(t, err)

	// 验证注册成功
	assert.Equal(t, 1, registry.Count())

	// 通过 connID 获取
	got := registry.GetByConnID("conn-1")
	assert.NotNil(t, got)
	assert.Equal(t, "conn-1", got.ConnID)

	// 通过 tunnelID 获取
	got = registry.GetByTunnelID("tunnel-1")
	assert.NotNil(t, got)
	assert.Equal(t, "tunnel-1", got.TunnelID)
}

func TestTunnelRegistry_UpdateAuth(t *testing.T) {
	registry := NewTunnelRegistry(&TunnelRegistryConfig{
		Logger: corelog.NewNopLogger(),
	})

	conn := &TunnelConnection{
		ConnID: "conn-1",
	}

	err := registry.Register(conn)
	require.NoError(t, err)

	// 更新认证信息
	err = registry.UpdateAuth("conn-1", "tunnel-1", "mapping-1")
	require.NoError(t, err)

	// 验证可以通过 tunnelID 获取
	got := registry.GetByTunnelID("tunnel-1")
	assert.NotNil(t, got)
	assert.Equal(t, "conn-1", got.ConnID)
	assert.Equal(t, "tunnel-1", got.TunnelID)
	assert.Equal(t, "mapping-1", got.MappingID)
	assert.True(t, got.Authenticated)
}

func TestTunnelRegistry_Remove(t *testing.T) {
	registry := NewTunnelRegistry(&TunnelRegistryConfig{
		Logger: corelog.NewNopLogger(),
	})

	conn := &TunnelConnection{
		ConnID:   "conn-1",
		TunnelID: "tunnel-1",
	}

	err := registry.Register(conn)
	require.NoError(t, err)

	// 移除连接
	registry.Remove("conn-1")

	// 验证已移除
	assert.Equal(t, 0, registry.Count())
	assert.Nil(t, registry.GetByConnID("conn-1"))
	assert.Nil(t, registry.GetByTunnelID("tunnel-1"))
}

func TestTunnelRegistry_List(t *testing.T) {
	registry := NewTunnelRegistry(&TunnelRegistryConfig{
		Logger: corelog.NewNopLogger(),
	})

	// 注册多个连接
	for i := 1; i <= 3; i++ {
		conn := &TunnelConnection{
			ConnID: "conn-" + string(rune('0'+i)),
		}
		err := registry.Register(conn)
		require.NoError(t, err)
	}

	// 列出所有连接
	list := registry.List()
	assert.Len(t, list, 3)
}

func TestTunnelRegistry_Close(t *testing.T) {
	registry := NewTunnelRegistry(&TunnelRegistryConfig{
		Logger: corelog.NewNopLogger(),
	})

	// 注册连接
	conn := &TunnelConnection{
		ConnID:   "conn-1",
		TunnelID: "tunnel-1",
	}
	err := registry.Register(conn)
	require.NoError(t, err)

	// 关闭注册表
	registry.Close()

	// 验证已清空
	assert.Equal(t, 0, registry.Count())
}
