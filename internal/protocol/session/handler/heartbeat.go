package handler

import (
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// HeartbeatManagerInterface SessionManager的心跳最小接口
type HeartbeatManagerInterface interface {
	// 连接查询和更新
	GetConnection(connID string) (*types.Connection, bool)
	UpdateConnectionHeartbeat(connID string, timestamp int64)
	UpdateControlConnectionActivity(connID string)
}

// HeartbeatHandler 心跳处理器
type HeartbeatHandler struct {
	sessionManager HeartbeatManagerInterface
	logger         corelog.Logger
}

// HeartbeatHandlerConfig 心跳处理器配置
type HeartbeatHandlerConfig struct {
	SessionManager HeartbeatManagerInterface
	Logger         corelog.Logger
}

// NewHeartbeatHandler 创建心跳处理器
func NewHeartbeatHandler(config *HeartbeatHandlerConfig) *HeartbeatHandler {
	if config == nil {
		config = &HeartbeatHandlerConfig{}
	}

	logger := config.Logger
	if logger == nil {
		logger = corelog.Default()
	}

	return &HeartbeatHandler{
		sessionManager: config.SessionManager,
		logger:         logger,
	}
}

// HandlePacket 处理心跳数据包
func (h *HeartbeatHandler) HandlePacket(connPacket *types.StreamPacket) error {
	return h.handleHeartbeat(connPacket)
}

// handleHeartbeat 处理心跳
func (h *HeartbeatHandler) handleHeartbeat(connPacket *types.StreamPacket) error {
	// 更新连接活跃时间
	timestamp := connPacket.Timestamp.Unix()
	h.sessionManager.UpdateConnectionHeartbeat(connPacket.ConnectionID, timestamp)

	// 更新 Control Connection 活跃时间
	h.sessionManager.UpdateControlConnectionActivity(connPacket.ConnectionID)

	h.logger.Debugf("Heartbeat received from connection: %s", connPacket.ConnectionID)

	// 发送心跳响应
	conn, exists := h.sessionManager.GetConnection(connPacket.ConnectionID)
	if exists && conn.Stream != nil {
		respPacket := &packet.TransferPacket{
			PacketType: packet.Heartbeat, // 心跳响应也用 Heartbeat
		}
		if _, err := conn.Stream.WritePacket(respPacket, true, 0); err != nil {
			h.logger.Errorf("Failed to send heartbeat response: %v", err)
		}
	}

	return nil
}
