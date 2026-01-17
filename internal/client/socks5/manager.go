// Package socks5 SOCKS5 映射管理器
// 管理 ClientA 端的 SOCKS5 代理监听器
package socks5

import (
	"context"
	"fmt"
	"sync"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
)

// Manager SOCKS5 映射管理器（运行在 ClientA）
type Manager struct {
	*dispose.ManagerBase

	listeners       map[string]*Listener
	mu              sync.RWMutex
	clientID        int64
	tunnelCreator   TunnelCreator
	udpRelayCreator UDPRelayCreator
}

// NewManager 创建 SOCKS5 管理器
func NewManager(ctx context.Context, clientID int64, tunnelCreator TunnelCreator) *Manager {
	m := &Manager{
		ManagerBase:   dispose.NewManager("SOCKS5Manager", ctx),
		listeners:     make(map[string]*Listener),
		clientID:      clientID,
		tunnelCreator: tunnelCreator,
	}

	m.AddCleanHandler(func() error {
		m.mu.Lock()
		defer m.mu.Unlock()

		for _, listener := range m.listeners {
			listener.Close()
		}
		m.listeners = make(map[string]*Listener)
		return nil
	})

	return m
}

// AddMapping 添加 SOCKS5 映射
// 只有当本客户端是 listen_client_id 时才启动监听器
func (m *Manager) AddMapping(mapping *models.PortMapping) error {
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
	config := &ListenerConfig{
		ListenAddr:     fmt.Sprintf(":%d", mapping.SourcePort),
		MappingID:      mapping.ID,
		TargetClientID: mapping.TargetClientID,
		SecretKey:      mapping.SecretKey,
	}

	listener := NewListener(m.Ctx(), config, m.tunnelCreator)

	corelog.Infof("SOCKS5Manager: AddMapping - udpRelayCreator=%v", m.udpRelayCreator != nil)
	if m.udpRelayCreator != nil {
		listener.SetUDPRelayCreator(m.udpRelayCreator)
		corelog.Infof("SOCKS5Manager: UDP relay creator set on listener")
	} else {
		corelog.Warnf("SOCKS5Manager: UDP relay creator is nil - UDP ASSOCIATE will not work!")
	}

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
func (m *Manager) RemoveMapping(mappingID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if listener, exists := m.listeners[mappingID]; exists {
		listener.Close()
		delete(m.listeners, mappingID)
		corelog.Infof("SOCKS5Manager: stopped listener for mapping %s", mappingID)
	}
}

// GetMapping 获取映射
func (m *Manager) GetMapping(mappingID string) (*Listener, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	listener, exists := m.listeners[mappingID]
	return listener, exists
}

// ListMappings 列出所有映射
func (m *Manager) ListMappings() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.listeners))
	for id := range m.listeners {
		ids = append(ids, id)
	}
	return ids
}

func (m *Manager) SetTunnelCreator(creator TunnelCreator) {
	m.tunnelCreator = creator
}

func (m *Manager) SetUDPRelayCreator(creator UDPRelayCreator) {
	m.udpRelayCreator = creator
}

func (m *Manager) SetClientID(clientID int64) {
	m.clientID = clientID
}
