package session

import (
	"context"
	"sync"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/events"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// SessionManager 会话管理器（双连接模型）
//
// 职责说明：
// 本文件仅包含核心协调逻辑，具体实现已按功能拆分：
//
//   - connection_lifecycle.go: 连接生命周期管理
//     CreateConnection, AcceptConnection, GetConnection, ListConnections,
//     UpdateConnectionState, CloseConnection, GetActiveConnections,
//     RegisterControlConnection, UpdateControlConnectionAuth, GetControlConnectionByClientID,
//     KickOldControlConnection, RemoveControlConnection,
//     RegisterTunnelConnection, UpdateTunnelConnectionAuth, GetTunnelConnectionByTunnelID,
//     GetTunnelConnectionByConnID, RemoveTunnelConnection, GetConnectionByClientID
//
//   - command_integration.go: Command 集成
//     SetEventBus, GetEventBus, GetResponseManager,
//     RegisterCommandHandler, UnregisterCommandHandler, ProcessCommand,
//     GetCommandRegistry, GetCommandExecutor, SetCommandExecutor,
//     handleCommandPacket, handleDefaultCommand, handleHeartbeat
//
//   - packet_handler.go: 数据包处理
//     HandlePacket, ProcessPacket,
//     handleHandshake, handleTunnelOpen
//
//   - event_handlers.go: 事件处理
//     handleDisconnectRequestEvent
type SessionManager struct {
	// 基础连接映射（所有连接）
	connMap  map[string]*types.Connection
	connLock sync.RWMutex

	// 指令连接（Control Connection）
	controlConnMap   map[string]*ControlConnection // connID -> 指令连接
	clientIDIndexMap map[int64]*ControlConnection  // clientID -> 指令连接（快速查找）
	controlConnLock  sync.RWMutex

	// 映射连接（Tunnel Connection）
	tunnelConnMap  map[string]*TunnelConnection // connID -> 映射连接
	tunnelIDMap    map[string]*TunnelConnection // tunnelID -> 映射连接
	tunnelConnLock sync.RWMutex

	idManager     *idgen.IDManager
	streamMgr     *stream.StreamManager
	streamFactory stream.StreamFactory

	// 事件驱动架构
	eventBus        events.EventBus
	responseManager *ResponseManager

	// Command集成
	commandRegistry types.CommandRegistry
	commandExecutor types.CommandExecutor

	// 隧道和认证处理器
	tunnelHandler TunnelHandler
	authHandler   AuthHandler

	// 临时兼容字段（迁移完成后删除）
	clientConnMap map[string]*ClientConnection

	dispose.Dispose
}

// NewSessionManager 创建新的会话管理器（双连接模型）
func NewSessionManager(idManager *idgen.IDManager, parentCtx context.Context) *SessionManager {
	// 创建默认流工厂
	streamFactory := stream.NewDefaultStreamFactory(parentCtx)

	// 创建流管理器
	streamMgr := stream.NewStreamManager(streamFactory, parentCtx)

	session := &SessionManager{
		// 基础连接
		connMap: make(map[string]*types.Connection),

		// 指令连接
		controlConnMap:   make(map[string]*ControlConnection),
		clientIDIndexMap: make(map[int64]*ControlConnection),

		// 映射连接
		tunnelConnMap: make(map[string]*TunnelConnection),
		tunnelIDMap:   make(map[string]*TunnelConnection),

		// 临时兼容
		clientConnMap: make(map[string]*ClientConnection),

		idManager:     idManager,
		streamMgr:     streamMgr,
		streamFactory: streamFactory,
		// 事件驱动架构将在后续设置
		eventBus:        nil,
		responseManager: nil,
		tunnelHandler:   nil,
		authHandler:     nil,
	}

	// 设置资源清理回调
	session.SetCtx(parentCtx, session.onClose)

	return session
}

// ============================================================================
// Handler 设置
// ============================================================================

// SetTunnelHandler 设置隧道处理器
func (s *SessionManager) SetTunnelHandler(handler TunnelHandler) {
	s.tunnelHandler = handler
	utils.Debug("Tunnel handler configured in SessionManager")
}

// SetAuthHandler 设置认证处理器
func (s *SessionManager) SetAuthHandler(handler AuthHandler) {
	s.authHandler = handler
	utils.Debug("Auth handler configured in SessionManager")
}

// ============================================================================
// Stream 管理
// ============================================================================

// GetStreamManager 获取流管理器
func (s *SessionManager) GetStreamManager() *stream.StreamManager {
	return s.streamMgr
}

// ============================================================================
// 资源清理
// ============================================================================

// onClose 资源清理回调
func (s *SessionManager) onClose() error {
	utils.Infof("Cleaning up session manager resources...")

	// 取消事件订阅
	if s.eventBus != nil {
		if err := s.eventBus.Unsubscribe("DisconnectRequest", s.handleDisconnectRequestEvent); err != nil {
			utils.Warnf("Failed to unsubscribe from DisconnectRequest events: %v", err)
		}
		utils.Infof("Unsubscribed from disconnect request events")
	}

	// 关闭所有连接
	s.connLock.Lock()
	for connID, conn := range s.connMap {
		if conn.Stream != nil {
			conn.Stream.Close()
		}
		utils.Debugf("Closed connection: %s", connID)
	}
	s.connMap = make(map[string]*types.Connection)
	s.connLock.Unlock()

	// 关闭所有控制连接
	s.controlConnLock.Lock()
	for connID, conn := range s.controlConnMap {
		if conn.Stream != nil {
			conn.Stream.Close()
		}
		utils.Debugf("Closed control connection: %s", connID)
	}
	s.controlConnMap = make(map[string]*ControlConnection)
	s.clientIDIndexMap = make(map[int64]*ControlConnection)
	s.controlConnLock.Unlock()

	// 关闭所有隧道连接
	s.tunnelConnLock.Lock()
	for connID, conn := range s.tunnelConnMap {
		if conn.Stream != nil {
			conn.Stream.Close()
		}
		utils.Debugf("Closed tunnel connection: %s", connID)
	}
	s.tunnelConnMap = make(map[string]*TunnelConnection)
	s.tunnelIDMap = make(map[string]*TunnelConnection)
	s.tunnelConnLock.Unlock()

	// 关闭流管理器
	if s.streamMgr != nil {
		utils.Debug("Stream manager resources cleaned up")
	}

	// 关闭事件总线
	if s.eventBus != nil {
		if err := s.eventBus.Close(); err != nil {
			utils.Errorf("Failed to close event bus: %v", err)
		}
		utils.Info("Event bus resources cleaned up")
	}

	utils.Info("Session manager resources cleanup completed")
	return nil
}
