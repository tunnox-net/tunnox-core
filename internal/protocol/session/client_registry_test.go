package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corelog "tunnox-core/internal/core/log"
)

func TestClientRegistry_Register(t *testing.T) {
	registry := NewClientRegistry(&ClientRegistryConfig{
		Logger: corelog.NewNopLogger(),
	})

	conn := &ControlConnection{
		ConnID:    "conn-1",
		ClientID:  1001,
		CreatedAt: time.Now(),
	}

	err := registry.Register(conn)
	require.NoError(t, err)

	// 验证注册成功
	assert.Equal(t, 1, registry.Count())

	// 通过 connID 获取
	got := registry.GetByConnID("conn-1")
	assert.NotNil(t, got)
	assert.Equal(t, "conn-1", got.ConnID)
}

func TestClientRegistry_UpdateAuth(t *testing.T) {
	registry := NewClientRegistry(&ClientRegistryConfig{
		Logger: corelog.NewNopLogger(),
	})

	conn := &ControlConnection{
		ConnID:    "conn-1",
		CreatedAt: time.Now(),
	}

	err := registry.Register(conn)
	require.NoError(t, err)

	// 更新认证信息
	err = registry.UpdateAuth("conn-1", 1001, "user-1")
	require.NoError(t, err)

	// 验证可以通过 clientID 获取
	got := registry.GetByClientID(1001)
	assert.NotNil(t, got)
	assert.Equal(t, "conn-1", got.ConnID)
	assert.Equal(t, int64(1001), got.ClientID)
	assert.Equal(t, "user-1", got.UserID)
	assert.True(t, got.Authenticated)
}

func TestClientRegistry_Remove(t *testing.T) {
	registry := NewClientRegistry(&ClientRegistryConfig{
		Logger: corelog.NewNopLogger(),
	})

	conn := &ControlConnection{
		ConnID:        "conn-1",
		ClientID:      1001,
		Authenticated: true,
		CreatedAt:     time.Now(),
	}

	err := registry.Register(conn)
	require.NoError(t, err)

	// 移除连接
	registry.Remove("conn-1")

	// 验证已移除
	assert.Equal(t, 0, registry.Count())
	assert.Nil(t, registry.GetByConnID("conn-1"))
	assert.Nil(t, registry.GetByClientID(1001))
}

func TestClientRegistry_MaxConnections(t *testing.T) {
	registry := NewClientRegistry(&ClientRegistryConfig{
		MaxConnections: 2,
		Logger:         corelog.NewNopLogger(),
	})

	// 注册第一个连接
	conn1 := &ControlConnection{
		ConnID:    "conn-1",
		CreatedAt: time.Now().Add(-2 * time.Second), // 最旧
	}
	err := registry.Register(conn1)
	require.NoError(t, err)

	// 注册第二个连接
	conn2 := &ControlConnection{
		ConnID:    "conn-2",
		CreatedAt: time.Now().Add(-1 * time.Second),
	}
	err = registry.Register(conn2)
	require.NoError(t, err)

	// 注册第三个连接（应该踢掉最旧的）
	conn3 := &ControlConnection{
		ConnID:    "conn-3",
		CreatedAt: time.Now(),
	}
	err = registry.Register(conn3)
	require.NoError(t, err)

	// 验证连接数
	assert.Equal(t, 2, registry.Count())

	// 验证最旧的连接被移除
	assert.Nil(t, registry.GetByConnID("conn-1"))
	assert.NotNil(t, registry.GetByConnID("conn-2"))
	assert.NotNil(t, registry.GetByConnID("conn-3"))
}

func TestClientRegistry_List(t *testing.T) {
	registry := NewClientRegistry(&ClientRegistryConfig{
		Logger: corelog.NewNopLogger(),
	})

	// 注册多个连接
	for i := 1; i <= 3; i++ {
		conn := &ControlConnection{
			ConnID:    "conn-" + string(rune('0'+i)),
			CreatedAt: time.Now(),
		}
		err := registry.Register(conn)
		require.NoError(t, err)
	}

	// 列出所有连接
	list := registry.List()
	assert.Len(t, list, 3)
}

func TestClientRegistry_Close(t *testing.T) {
	registry := NewClientRegistry(&ClientRegistryConfig{
		Logger: corelog.NewNopLogger(),
	})

	// 注册连接
	conn := &ControlConnection{
		ConnID:    "conn-1",
		CreatedAt: time.Now(),
	}
	err := registry.Register(conn)
	require.NoError(t, err)

	// 关闭注册表
	registry.Close()

	// 验证已清空
	assert.Equal(t, 0, registry.Count())
}
