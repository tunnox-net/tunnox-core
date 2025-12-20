package server

import (
	"fmt"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/command"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
)

// ConfigCommandHandlers 配置命令处理器集合
type ConfigCommandHandlers struct {
	authHandler *ServerAuthHandler
	sessionMgr  *session.SessionManager
}

// NewConfigCommandHandlers 创建配置命令处理器
func NewConfigCommandHandlers(
	authHandler *ServerAuthHandler,
	sessionMgr *session.SessionManager,
) *ConfigCommandHandlers {
	return &ConfigCommandHandlers{
		authHandler: authHandler,
		sessionMgr:  sessionMgr,
	}
}

// RegisterHandlers 注册所有配置命令处理器
func (h *ConfigCommandHandlers) RegisterHandlers(registry *command.CommandRegistry) error {
	if registry == nil {
		return fmt.Errorf("command registry is nil")
	}

	// 注册 ConfigGet 命令
	configGetHandler := &ConfigGetHandler{
		BaseHandler: command.NewBaseHandler(
			packet.ConfigGet,
			command.CategoryManagement,
			command.DirectionDuplex,
			"config_get",
			"获取配置信息",
		),
		authHandler: h.authHandler,
		sessionMgr:  h.sessionMgr,
	}
	if err := registry.Register(configGetHandler); err != nil {
		return fmt.Errorf("failed to register config get handler: %w", err)
	}

	return nil
}

// ConfigGetHandler ConfigGet 命令处理器
type ConfigGetHandler struct {
	*command.BaseHandler
	authHandler *ServerAuthHandler
	sessionMgr  *session.SessionManager
}

// Handle 处理 ConfigGet 命令
func (h *ConfigGetHandler) Handle(ctx *command.CommandContext) (*command.CommandResponse, error) {
	// 获取客户端ID
	clientID := h.getClientID(ctx)
	if clientID == 0 {
		return h.errorResponse(ctx, "client not authenticated")
	}

	// 获取控制连接
	controlConn := h.sessionMgr.GetControlConnection(ctx.ConnectionID)
	if controlConn == nil {
		return h.errorResponse(ctx, "control connection not found")
	}

	// 调用 GetClientConfig 获取配置
	configJSON, err := h.authHandler.GetClientConfig(controlConn)
	if err != nil {
		corelog.Errorf("ConfigGetHandler: failed to get client config for client %d: %v", clientID, err)
		return h.errorResponse(ctx, fmt.Sprintf("failed to get config: %v", err))
	}

	return &command.CommandResponse{
		Success:   true,
		Data:      configJSON,
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// getClientID 从上下文中获取客户端ID
func (h *ConfigGetHandler) getClientID(ctx *command.CommandContext) int64 {
	if h.sessionMgr == nil {
		return 0
	}
	controlConn := h.sessionMgr.GetControlConnection(ctx.ConnectionID)
	if controlConn == nil {
		return 0
	}
	return controlConn.ClientID
}

// errorResponse 构造错误响应
func (h *ConfigGetHandler) errorResponse(ctx *command.CommandContext, message string) (*command.CommandResponse, error) {
	return &command.CommandResponse{
		Success:   false,
		Error:     message,
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}
