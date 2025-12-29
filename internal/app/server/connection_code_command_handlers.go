package server

import (
	"encoding/json"
	"fmt"
	"time"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/command"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
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
	// 获取客户端ID
	clientID := h.getClientID(ctx)
	if clientID == 0 {
		return h.errorResponse(ctx, "client not authenticated")
	}

	// 解析请求
	var req struct {
		TargetAddress string `json:"target_address"`
		ActivationTTL int    `json:"activation_ttl"` // 秒
		MappingTTL    int    `json:"mapping_ttl"`    // 秒
		Description   string `json:"description,omitempty"`
	}

	if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
		corelog.Errorf("GenerateConnectionCodeHandler: failed to parse request: %v", err)
		return h.errorResponse(ctx, fmt.Sprintf("invalid request: %v", err))
	}

	// 调用服务
	connCode, err := h.connCodeService.CreateConnectionCode(&services.CreateConnectionCodeRequest{
		TargetClientID:  clientID,
		TargetAddress:   req.TargetAddress,
		ActivationTTL:   time.Duration(req.ActivationTTL) * time.Second,
		MappingDuration: time.Duration(req.MappingTTL) * time.Second,
		Description:     req.Description,
		CreatedBy:       fmt.Sprintf("client-%d", clientID),
	})

	if err != nil {
		corelog.Errorf("GenerateConnectionCodeHandler: failed to create connection code: %v", err)
		return h.errorResponse(ctx, fmt.Sprintf("failed to create connection code: %v", err))
	}

	// 构造响应
	resp := ConnectionCodeResponse{
		Code:          connCode.Code,
		TargetAddress: connCode.TargetAddress,
		ExpiresAt:     connCode.ActivationExpiresAt.Format(time.RFC3339),
		Description:   connCode.Description,
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

	// 构造响应（过滤掉已过期的连接码）
	codeInfos := make([]ConnectionCodeInfo, 0, len(codes))
	for _, code := range codes {
		// 跳过已过期的连接码（不显示）
		if code.IsExpired() && !code.IsActivated {
			// 已过期且未激活的连接码不显示，但已激活的可以显示（用于查看历史）
			continue
		}

		status := "available"
		if code.IsRevoked {
			status = "revoked"
		} else if code.IsActivated {
			status = "activated"
		}

		activatedByStr := ""
		if code.ActivatedBy != nil {
			activatedByStr = fmt.Sprintf("%d", *code.ActivatedBy)
		}
		codeInfo := ConnectionCodeInfo{
			Code:          code.Code,
			TargetAddress: code.TargetAddress,
			Status:        status,
			CreatedAt:     code.CreatedAt.Format(time.RFC3339),
			ExpiresAt:     code.ActivationExpiresAt.Format(time.RFC3339),
			Activated:     code.IsActivated,
			ActivatedBy:   activatedByStr,
			Description:   code.Description,
		}
		codeInfos = append(codeInfos, codeInfo)
	}

	resp := ConnectionCodeListResponse{
		Codes: codeInfos,
		Total: len(codeInfos),
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

	resp := MappingActivateResponse{
		MappingID:      mapping.ID,
		TargetAddress:  mapping.TargetAddress,
		ListenAddress:  mapping.ListenAddress,
		ExpiresAt:      expiresAtStr,
		TargetClientID: mapping.TargetClientID, // SOCKS5 映射需要
		SecretKey:      mapping.SecretKey,      // SOCKS5 映射需要
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
	if h.sessionMgr == nil {
		return 0
	}
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
