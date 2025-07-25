package protocol

import (
	"context"
	"fmt"
	"tunnox-core/internal/protocol/adapter"
	"tunnox-core/internal/utils"
)

// ProtocolService 协议服务适配器，让协议管理器能够作为服务运行
type ProtocolService struct {
	manager *ProtocolManager
	name    string
	ctx     context.Context
}

// NewProtocolService 创建协议服务
func NewProtocolService(name string, manager *ProtocolManager) *ProtocolService {
	return &ProtocolService{
		manager: manager,
		name:    name,
	}
}

// Name 实现Service接口
func (ps *ProtocolService) Name() string {
	return ps.name
}

// Start 启动协议服务
func (ps *ProtocolService) Start(ctx context.Context) error {
	ps.ctx = ctx
	utils.Infof("Starting protocol service: %s", ps.name)

	// 启动所有协议适配器
	if err := ps.manager.StartAll(); err != nil {
		return fmt.Errorf("failed to start protocol service %s: %v", ps.name, err)
	}

	utils.Infof("Protocol service started: %s", ps.name)
	return nil
}

// Stop 停止协议服务
func (ps *ProtocolService) Stop(ctx context.Context) error {
	utils.Infof("Stopping protocol service: %s", ps.name)

	// 关闭所有协议适配器
	if err := ps.manager.CloseAll(); err != nil {
		return fmt.Errorf("failed to stop protocol service %s: %v", ps.name, err)
	}

	utils.Infof("Protocol service stopped: %s", ps.name)
	return nil
}

// GetManager 获取协议管理器
func (ps *ProtocolService) GetManager() *ProtocolManager {
	return ps.manager
}

// RegisterAdapter 注册协议适配器
func (ps *ProtocolService) RegisterAdapter(adapter adapter.Adapter) {
	ps.manager.Register(adapter)
}

// GetAdapterCount 获取适配器数量
func (ps *ProtocolService) GetAdapterCount() int {
	return len(ps.manager.adapters)
}
