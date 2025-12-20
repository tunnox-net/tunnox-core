// Package client SOCKS5 映射管理器
// 管理 ClientA 端的 SOCKS5 代理监听器
package client

import (
	"context"
	"fmt"
	"sync"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
)

// SOCKS5Manager SOCKS5 映射管理器（运行在 ClientA）
type SOCKS5Manager struct {
	*dispose.ManagerBase

	listeners     map[string]*SOCKS5Listener // mappingID -> listener
	mu            sync.RWMutex
	clientID      int64               // 本客户端ID
	tunnelCreator SOCKS5TunnelCreator // 隧道创建器
}

// NewSOCKS5Manager 创建 SOCKS5 管理器
func NewSOCKS5Manager(ctx context.Context, clientID int64, tunnelCreator SOCKS5TunnelCreator) *SOCKS5Manager {
	m := &SOCKS5Manager{
		ManagerBase:   dispose.NewManager("SOCKS5Manager", ctx),
		listeners:     make(map[string]*SOCKS5Listener),
		clientID:      clientID,
		tunnelCreator: tunnelCreator,
	}

	m.AddCleanHandler(func() error {
		m.mu.Lock()
		defer m.mu.Unlock()

		for _, listener := range m.listeners {
			listener.Close()
		}
		m.listeners = make(map[string]*SOCKS5Listener)
		return nil
	})

	return m
}

// AddMapping 添加 SOCKS5 映射
// 只有当本客户端是 listen_client_id 时才启动监听器
func (m *SOCKS5Manager) AddMapping(mapping *models.PortMapping) error {
	if mapping.Protocol != models.ProtocolSOCKS {
		return nil // 非 SOCKS5 映射，忽略
	}

	// 只有当本客户端是 listen_client_id 时才启动监听器
	if mapping.ListenClientID != m.clientID {
		corelog.Debugf("SOCKS5Manager: mapping %s is not for this client (listen=%d, this=%d)",
			mapping.ID, mapping.ListenClientID, m.clientID)
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已存在
	if _, exists := m.listeners[mapping.ID]; exists {
		corelog.Debugf("SOCKS5Manager: mapping %s already exists", mapping.ID)
		return nil
	}

	// 创建监听器配置
	config := &SOCKS5ListenerConfig{
		ListenAddr:     fmt.Sprintf(":%d", mapping.SourcePort),
		MappingID:      mapping.ID,
		TargetClientID: mapping.TargetClientID,
		SecretKey:      mapping.SecretKey,
	}

	// 创建监听器
	listener := NewSOCKS5Listener(m.Ctx(), config, m.tunnelCreator)

	// 启动监听
	if err := listener.Start(); err != nil {
		return err
	}

	m.listeners[mapping.ID] = listener
	corelog.Infof("SOCKS5Manager: started listener on port %d for mapping %s (target client: %d)",
		mapping.SourcePort, mapping.ID, mapping.TargetClientID)

	return nil
}

// RemoveMapping 移除 SOCKS5 映射
func (m *SOCKS5Manager) RemoveMapping(mappingID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if listener, exists := m.listeners[mappingID]; exists {
		listener.Close()
		delete(m.listeners, mappingID)
		corelog.Infof("SOCKS5Manager: stopped listener for mapping %s", mappingID)
	}
}

// GetMapping 获取映射
func (m *SOCKS5Manager) GetMapping(mappingID string) (*SOCKS5Listener, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	listener, exists := m.listeners[mappingID]
	return listener, exists
}

// ListMappings 列出所有映射
func (m *SOCKS5Manager) ListMappings() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.listeners))
	for id := range m.listeners {
		ids = append(ids, id)
	}
	return ids
}

// SetTunnelCreator 设置隧道创建器
func (m *SOCKS5Manager) SetTunnelCreator(creator SOCKS5TunnelCreator) {
	m.tunnelCreator = creator
}

// SetClientID 设置客户端ID（认证后调用）
func (m *SOCKS5Manager) SetClientID(clientID int64) {
	m.clientID = clientID
}
