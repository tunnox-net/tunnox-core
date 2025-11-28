package server

import (
	"encoding/json"
	"fmt"
	"time"

	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/command"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码命令处理器
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ConnectionCodeCommandHandlers 连接码命令处理器集合
type ConnectionCodeCommandHandlers struct {
	connCodeService *services.ConnectionCodeService
	sessionMgr      *session.SessionManager
}

// NewConnectionCodeCommandHandlers 创建连接码命令处理器
func NewConnectionCodeCommandHandlers(
	connCodeService *services.ConnectionCodeService,
	sessionMgr *session.SessionManager,
) *ConnectionCodeCommandHandlers {
	return &ConnectionCodeCommandHandlers{
		connCodeService: connCodeService,
		sessionMgr:      sessionMgr,
	}
}

// RegisterHandlers 注册所有连接码命令处理器
func (h *ConnectionCodeCommandHandlers) RegisterHandlers(registry *command.CommandRegistry) error {
	if registry == nil {
		return fmt.Errorf("command registry is nil")
	}

	// 注册生成连接码命令
	generateHandler := &GenerateConnectionCodeHandler{
		BaseHandler: command.NewBaseHandler(
			packet.ConnectionCodeGenerate,
			command.CategoryMapping,
			command.DirectionDuplex,
			"connection_code_generate",
			"生成连接码",
		),
		connCodeService: h.connCodeService,
		sessionMgr:      h.sessionMgr,
	}
	if err := registry.Register(generateHandler); err != nil {
		return fmt.Errorf("failed to register generate handler: %w", err)
	}

	// 注册列出连接码命令
	listHandler := &ListConnectionCodesHandler{
		BaseHandler: command.NewBaseHandler(
			packet.ConnectionCodeList,
			command.CategoryMapping,
			command.DirectionDuplex,
			"connection_code_list",
			"列出连接码",
		),
		connCodeService: h.connCodeService,
		sessionMgr:      h.sessionMgr,
	}
	if err := registry.Register(listHandler); err != nil {
		return fmt.Errorf("failed to register list handler: %w", err)
	}

	// 注册激活连接码命令
	activateHandler := &ActivateConnectionCodeHandler{
		BaseHandler: command.NewBaseHandler(
			packet.ConnectionCodeActivate,
			command.CategoryMapping,
			command.DirectionDuplex,
			"connection_code_activate",
			"激活连接码",
		),
		connCodeService: h.connCodeService,
		sessionMgr:      h.sessionMgr,
	}
	if err := registry.Register(activateHandler); err != nil {
		return fmt.Errorf("failed to register activate handler: %w", err)
	}

	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 生成连接码处理器
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GenerateConnectionCodeHandler 生成连接码命令处理器
type GenerateConnectionCodeHandler struct {
	*command.BaseHandler
	connCodeService *services.ConnectionCodeService
	sessionMgr      *session.SessionManager
}

// Handle 处理生成连接码命令
func (h *GenerateConnectionCodeHandler) Handle(ctx *command.CommandContext) (*command.CommandResponse, error) {
	utils.Debugf("GenerateConnectionCodeHandler: handling command, ConnectionID=%s, CommandID=%s", ctx.ConnectionID, ctx.CommandId)

	// 获取客户端ID
	clientID := h.getClientID(ctx)
	if clientID == 0 {
		utils.Warnf("GenerateConnectionCodeHandler: client not authenticated for connection %s", ctx.ConnectionID)
		return h.errorResponse(ctx, "client not authenticated")
	}

	utils.Debugf("GenerateConnectionCodeHandler: clientID=%d, requestBody=%s", clientID, ctx.RequestBody)

	// 解析请求
	var req struct {
		TargetAddress string `json:"target_address"`
		ActivationTTL int    `json:"activation_ttl"` // 秒
		MappingTTL    int    `json:"mapping_ttl"`    // 秒
		Description   string `json:"description,omitempty"`
	}

	if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
		utils.Errorf("GenerateConnectionCodeHandler: failed to parse request: %v", err)
		return h.errorResponse(ctx, fmt.Sprintf("invalid request: %v", err))
	}

	utils.Debugf("GenerateConnectionCodeHandler: parsed request - TargetAddress=%s, ActivationTTL=%d, MappingTTL=%d", req.TargetAddress, req.ActivationTTL, req.MappingTTL)

	// 调用服务
	utils.Debugf("GenerateConnectionCodeHandler: calling CreateConnectionCode service")
	connCode, err := h.connCodeService.CreateConnectionCode(&services.CreateConnectionCodeRequest{
		TargetClientID:  clientID,
		TargetAddress:   req.TargetAddress,
		ActivationTTL:   time.Duration(req.ActivationTTL) * time.Second,
		MappingDuration: time.Duration(req.MappingTTL) * time.Second,
		Description:     req.Description,
		CreatedBy:       fmt.Sprintf("client-%d", clientID),
	})

	if err != nil {
		utils.Errorf("GenerateConnectionCodeHandler: CreateConnectionCode failed: %v", err)
		return h.errorResponse(ctx, fmt.Sprintf("failed to create connection code: %v", err))
	}

	utils.Infof("GenerateConnectionCodeHandler: connection code created successfully, Code=%s", connCode.Code)

	// 构造响应
	resp := map[string]interface{}{
		"code":           connCode.Code,
		"target_address": connCode.TargetAddress,
		"expires_at":     connCode.ActivationExpiresAt.Format(time.RFC3339),
		"description":    connCode.Description,
	}

	respBody, _ := json.Marshal(resp)
	utils.Debugf("GenerateConnectionCodeHandler: response prepared, CommandID=%s, ResponseSize=%d", ctx.CommandId, len(respBody))
	return &command.CommandResponse{
		Success:   true,
		Data:      string(respBody),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 列出连接码处理器
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ListConnectionCodesHandler 列出连接码命令处理器
type ListConnectionCodesHandler struct {
	*command.BaseHandler
	connCodeService *services.ConnectionCodeService
	sessionMgr      *session.SessionManager
}

// Handle 处理列出连接码命令
func (h *ListConnectionCodesHandler) Handle(ctx *command.CommandContext) (*command.CommandResponse, error) {
	// 获取客户端ID
	clientID := h.getClientID(ctx)
	if clientID == 0 {
		return h.errorResponse(ctx, "client not authenticated")
	}

	// 调用服务
	codes, err := h.connCodeService.ListConnectionCodesByTargetClient(clientID)
	if err != nil {
		return h.errorResponse(ctx, fmt.Sprintf("failed to list connection codes: %v", err))
	}

	// 构造响应
	codeInfos := make([]map[string]interface{}, 0, len(codes))
	for _, code := range codes {
		status := "active"
		if code.IsRevoked {
			status = "revoked"
		} else if code.IsActivated {
			status = "activated"
		} else if code.IsExpired() {
			status = "expired"
		}

		codeInfo := map[string]interface{}{
			"code":           code.Code,
			"target_address": code.TargetAddress,
			"status":         status,
			"created_at":     code.CreatedAt.Format(time.RFC3339),
			"expires_at":     code.ActivationExpiresAt.Format(time.RFC3339),
			"activated":      code.IsActivated,
			"description":    code.Description,
		}
		codeInfos = append(codeInfos, codeInfo)
	}

	resp := map[string]interface{}{
		"codes": codeInfos,
		"total": len(codeInfos),
	}

	respBody, _ := json.Marshal(resp)
	return &command.CommandResponse{
		Success:   true,
		Data:      string(respBody),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 激活连接码处理器
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ActivateConnectionCodeHandler 激活连接码命令处理器
type ActivateConnectionCodeHandler struct {
	*command.BaseHandler
	connCodeService *services.ConnectionCodeService
	sessionMgr      *session.SessionManager
}

// Handle 处理激活连接码命令
func (h *ActivateConnectionCodeHandler) Handle(ctx *command.CommandContext) (*command.CommandResponse, error) {
	// 获取客户端ID
	clientID := h.getClientID(ctx)
	if clientID == 0 {
		return h.errorResponse(ctx, "client not authenticated")
	}

	// 解析请求
	var req struct {
		Code          string `json:"code"`
		ListenAddress string `json:"listen_address"`
	}

	if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
		return h.errorResponse(ctx, fmt.Sprintf("invalid request: %v", err))
	}

	// 调用服务
	mapping, err := h.connCodeService.ActivateConnectionCode(&services.ActivateConnectionCodeRequest{
		Code:           req.Code,
		ListenClientID: clientID,
		ListenAddress:  req.ListenAddress,
	})

	if err != nil {
		return h.errorResponse(ctx, fmt.Sprintf("failed to activate connection code: %v", err))
	}

	// 构造响应
	expiresAtStr := ""
	if mapping.ExpiresAt != nil {
		expiresAtStr = mapping.ExpiresAt.Format(time.RFC3339)
	}

	resp := map[string]interface{}{
		"mapping_id":     mapping.ID,
		"target_address": mapping.TargetAddress,
		"listen_address": mapping.ListenAddress,
		"expires_at":     expiresAtStr,
	}

	respBody, _ := json.Marshal(resp)
	return &command.CommandResponse{
		Success:   true,
		Data:      string(respBody),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 辅助方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// getClientID 从上下文中获取客户端ID
func (h *ConnectionCodeCommandHandlers) getClientID(ctx *command.CommandContext) int64 {
	// 从 SessionManager 获取控制连接
	if h.sessionMgr == nil {
		return 0
	}

	// 通过 ConnectionID 获取控制连接
	controlConn := h.sessionMgr.GetControlConnection(ctx.ConnectionID)
	if controlConn == nil {
		return 0
	}

	return controlConn.ClientID
}

// getClientID 获取客户端ID（处理器方法）
func (h *GenerateConnectionCodeHandler) getClientID(ctx *command.CommandContext) int64 {
	if h.sessionMgr == nil {
		return 0
	}
	controlConn := h.sessionMgr.GetControlConnection(ctx.ConnectionID)
	if controlConn == nil {
		return 0
	}
	return controlConn.ClientID
}

func (h *ListConnectionCodesHandler) getClientID(ctx *command.CommandContext) int64 {
	if h.sessionMgr == nil {
		return 0
	}
	controlConn := h.sessionMgr.GetControlConnection(ctx.ConnectionID)
	if controlConn == nil {
		return 0
	}
	return controlConn.ClientID
}

func (h *ActivateConnectionCodeHandler) getClientID(ctx *command.CommandContext) int64 {
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
func (h *GenerateConnectionCodeHandler) errorResponse(ctx *command.CommandContext, message string) (*command.CommandResponse, error) {
	return &command.CommandResponse{
		Success:   false,
		Error:     message,
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

func (h *ListConnectionCodesHandler) errorResponse(ctx *command.CommandContext, message string) (*command.CommandResponse, error) {
	return &command.CommandResponse{
		Success:   false,
		Error:     message,
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

func (h *ActivateConnectionCodeHandler) errorResponse(ctx *command.CommandContext, message string) (*command.CommandResponse, error) {
	return &command.CommandResponse{
		Success:   false,
		Error:     message,
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}
