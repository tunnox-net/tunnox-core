package client

import (
	"context"
	"sync"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
	"net"
)

// TunnoxClient 隧道客户端
type TunnoxClient struct {
	*dispose.ManagerBase

	config *ClientConfig

	// 指令连接
	controlConn   net.Conn
	controlStream stream.PackageStreamer

	// 映射管理
	mappingHandlers map[string]MappingHandler
	mu              sync.RWMutex

	// 商业化控制：配额缓存
	cachedQuota      *models.UserQuota
	quotaCacheMu     sync.RWMutex
	quotaLastRefresh time.Time

	// 商业化控制：流量累计
	localTrafficStats map[string]*localMappingStats // mappingID -> stats
	trafficStatsMu    sync.RWMutex

	// API客户端（用于CLI调用Management API）
	apiClient *ManagementAPIClient

	// 重连控制
	kicked     bool // 是否被踢下线
	authFailed bool // 是否认证失败
}

// localMappingStats 本地映射流量统计
type localMappingStats struct {
	bytesSent      int64
	bytesReceived  int64
	lastReportTime time.Time
	mu             sync.RWMutex
}

// NewClient 创建客户端
func NewClient(ctx context.Context, config *ClientConfig) *TunnoxClient {
	client := &TunnoxClient{
		ManagerBase:       dispose.NewManager("TunnoxClient", ctx),
		config:            config,
		mappingHandlers:   make(map[string]MappingHandler),
		localTrafficStats: make(map[string]*localMappingStats),
	}

	// 初始化API客户端（用于CLI）
	// 假设Management API在服务器地址的8080端口
	managementAPIAddr := config.Server.Address
	if managementAPIAddr == "" {
		managementAPIAddr = "localhost:8080"
	}
	client.apiClient = NewManagementAPIClient(managementAPIAddr, config.ClientID, config.AuthToken)

	// 添加清理处理器
	client.AddCleanHandler(func() error {
		utils.Infof("Client: cleaning up client resources")

		// 关闭所有映射处理器
		client.mu.RLock()
		handlers := make([]MappingHandler, 0, len(client.mappingHandlers))
		for _, handler := range client.mappingHandlers {
			handlers = append(handlers, handler)
		}
		client.mu.RUnlock()

		for _, handler := range handlers {
			handler.Stop()
		}

		// 关闭控制连接
		if client.controlConn != nil {
			client.controlConn.Close()
		}

		return nil
	})

	return client
}

// Stop 停止客户端
func (c *TunnoxClient) Stop() {
	utils.Infof("Client: stopping...")
	c.Close()
}

// GetContext 获取上下文（供映射处理器使用）
func (c *TunnoxClient) GetContext() context.Context {
	return c.Ctx()
}

// GetConfig 获取配置（供映射处理器使用）
func (c *TunnoxClient) GetConfig() *ClientConfig {
	return c.config
}


// GetAPIClient 获取Management API客户端（供CLI使用）
func (c *TunnoxClient) GetAPIClient() *ManagementAPIClient {
	return c.apiClient
}
