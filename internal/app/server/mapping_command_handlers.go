package server

import (
	"encoding/json"
	"fmt"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/command"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
)

// MappingCommandHandlers 映射命令处理器集合
type MappingCommandHandlers struct {
	connCodeService *services.ConnectionCodeService
	sessionMgr      *session.SessionManager
}

// NewMappingCommandHandlers 创建映射命令处理器
func NewMappingCommandHandlers(
	connCodeService *services.ConnectionCodeService,
	sessionMgr *session.SessionManager,
) *MappingCommandHandlers {
	return &MappingCommandHandlers{
		connCodeService: connCodeService,
		sessionMgr:      sessionMgr,
	}
}

// RegisterHandlers 注册所有映射命令处理器
func (h *MappingCommandHandlers) RegisterHandlers(registry *command.CommandRegistry) error {
	if registry == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "command registry is nil")
	}

	listHandler := &ListMappingsHandler{
		BaseHandler: command.NewBaseHandler(
			packet.MappingList,
			command.CategoryMapping,
			command.DirectionDuplex,
			"mapping_list",
			"列出映射列表",
		),
		connCodeService: h.connCodeService,
		sessionMgr:      h.sessionMgr,
	}
	if err := registry.Register(listHandler); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to register list mappings handler")
	}

	getHandler := &GetMappingHandler{
		BaseHandler: command.NewBaseHandler(
			packet.MappingGet,
			command.CategoryMapping,
			command.DirectionDuplex,
			"mapping_get",
			"获取映射详情",
		),
		connCodeService: h.connCodeService,
		sessionMgr:      h.sessionMgr,
	}
	if err := registry.Register(getHandler); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to register get mapping handler")
	}

	deleteHandler := &DeleteMappingHandler{
		BaseHandler: command.NewBaseHandler(
			packet.MappingDelete,
			command.CategoryMapping,
			command.DirectionDuplex,
			"mapping_delete",
			"删除映射",
		),
		connCodeService: h.connCodeService,
		sessionMgr:      h.sessionMgr,
	}
	if err := registry.Register(deleteHandler); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to register delete mapping handler")
	}

	return nil
}

// ListMappingsHandler 列出映射命令处理器
type ListMappingsHandler struct {
	*command.BaseHandler
	connCodeService *services.ConnectionCodeService
	sessionMgr      *session.SessionManager
}

// Handle 处理 ListMappings 命令
func (h *ListMappingsHandler) Handle(ctx *command.CommandContext) (*command.CommandResponse, error) {
	// 获取客户端ID
	clientID := h.getClientID(ctx)
	if clientID == 0 {
		return h.errorResponse(ctx, "client not authenticated")
	}

	var req struct {
		Direction string `json:"direction"`
		Type      string `json:"type"`
		Status    string `json:"status"`
	}

	if ctx.RequestBody != "" {
		if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
			corelog.Warnf("ListMappingsHandler: failed to parse request body: %v", err)
		}
	}

	// 获取映射列表
	var mappings []*models.PortMapping
	var err error

	switch req.Direction {
	case "outbound":
		mappings, err = h.connCodeService.ListOutboundMappings(clientID)
	case "inbound":
		mappings, err = h.connCodeService.ListInboundMappings(clientID)
	default:
		outboundMappings, err1 := h.connCodeService.ListOutboundMappings(clientID)
		inboundMappings, err2 := h.connCodeService.ListInboundMappings(clientID)
		if err1 != nil {
			err = err1
		} else if err2 != nil {
			err = err2
		} else {
			mappingMap := make(map[string]*models.PortMapping)
			for _, m := range outboundMappings {
				mappingMap[m.ID] = m
			}
			for _, m := range inboundMappings {
				mappingMap[m.ID] = m
			}
			mappings = make([]*models.PortMapping, 0, len(mappingMap))
			for _, m := range mappingMap {
				mappings = append(mappings, m)
			}
		}
	}

	if err != nil {
		corelog.Errorf("ListMappingsHandler: failed to list mappings for client %d: %v", clientID, err)
		return h.errorResponse(ctx, fmt.Sprintf("failed to list mappings: %v", err))
	}

	mappingItems := make([]MappingItem, 0, len(mappings))
	for _, m := range mappings {
		if req.Status != "" && string(m.Status) != req.Status {
			continue
		}
		if req.Type != "" && string(m.Protocol) != req.Type {
			continue
		}

		mappingType := "outbound"
		if m.TargetClientID == clientID && m.ListenClientID != clientID {
			mappingType = "inbound"
		}

		expiresAtStr := ""
		if m.ExpiresAt != nil {
			expiresAtStr = m.ExpiresAt.Format(time.RFC3339)
		}

		item := MappingItem{
			MappingID:     m.ID,
			Type:          mappingType,
			TargetAddress: m.TargetAddress,
			ListenAddress: m.ListenAddress,
			Status:        string(m.Status),
			ExpiresAt:     expiresAtStr,
			CreatedAt:     m.CreatedAt.Format(time.RFC3339),
			BytesSent:     m.TrafficStats.BytesSent,
			BytesReceived: m.TrafficStats.BytesReceived,
		}

		mappingItems = append(mappingItems, item)
	}

	resp := MappingListResponse{
		Mappings: mappingItems,
		Total:    len(mappingItems),
	}

	respBody, err := json.Marshal(resp)
	if err != nil {
		corelog.Errorf("ListMappingsHandler: failed to marshal response: %v", err)
		return h.errorResponse(ctx, "failed to serialize response")
	}
	return &command.CommandResponse{
		Success:   true,
		Data:      string(respBody),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// getClientID 从上下文中获取客户端ID
func (h *ListMappingsHandler) getClientID(ctx *command.CommandContext) int64 {
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
func (h *ListMappingsHandler) errorResponse(ctx *command.CommandContext, message string) (*command.CommandResponse, error) {
	return &command.CommandResponse{
		Success:   false,
		Error:     message,
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// GetMappingHandler 获取映射详情命令处理器
type GetMappingHandler struct {
	*command.BaseHandler
	connCodeService *services.ConnectionCodeService
	sessionMgr      *session.SessionManager
}

// Handle 处理 GetMapping 命令
func (h *GetMappingHandler) Handle(ctx *command.CommandContext) (*command.CommandResponse, error) {
	clientID := h.getClientID(ctx)
	if clientID == 0 {
		return h.errorResponse(ctx, "client not authenticated")
	}

	var req struct {
		MappingID string `json:"mapping_id"`
	}

	if ctx.RequestBody != "" {
		if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
			return h.errorResponse(ctx, fmt.Sprintf("failed to parse request: %v", err))
		}
	}

	if req.MappingID == "" {
		return h.errorResponse(ctx, "mapping_id is required")
	}

	mapping, err := h.connCodeService.GetMapping(req.MappingID)
	if err != nil {
		return h.errorResponse(ctx, fmt.Sprintf("mapping not found: %v", err))
	}

	if mapping.ListenClientID != clientID && mapping.TargetClientID != clientID {
		return h.errorResponse(ctx, "mapping not accessible")
	}

	mappingType := "outbound"
	if mapping.TargetClientID == clientID && mapping.ListenClientID != clientID {
		mappingType = "inbound"
	}

	expiresAtStr := ""
	if mapping.ExpiresAt != nil {
		expiresAtStr = mapping.ExpiresAt.Format(time.RFC3339)
	}

	item := MappingItem{
		MappingID:     mapping.ID,
		Type:          mappingType,
		TargetAddress: mapping.TargetAddress,
		ListenAddress: mapping.ListenAddress,
		Status:        string(mapping.Status),
		ExpiresAt:     expiresAtStr,
		CreatedAt:     mapping.CreatedAt.Format(time.RFC3339),
		BytesSent:     mapping.TrafficStats.BytesSent,
		BytesReceived: mapping.TrafficStats.BytesReceived,
	}

	resp := MappingDetailResponse{Mapping: item}
	respBody, err := json.Marshal(resp)
	if err != nil {
		corelog.Errorf("GetMappingHandler: failed to marshal response: %v", err)
		return h.errorResponse(ctx, "failed to serialize response")
	}
	return &command.CommandResponse{
		Success:   true,
		Data:      string(respBody),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

func (h *GetMappingHandler) getClientID(ctx *command.CommandContext) int64 {
	if h.sessionMgr == nil {
		return 0
	}
	controlConn := h.sessionMgr.GetControlConnection(ctx.ConnectionID)
	if controlConn == nil {
		return 0
	}
	return controlConn.ClientID
}

func (h *GetMappingHandler) errorResponse(ctx *command.CommandContext, message string) (*command.CommandResponse, error) {
	return &command.CommandResponse{
		Success:   false,
		Error:     message,
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// DeleteMappingHandler 删除映射命令处理器
type DeleteMappingHandler struct {
	*command.BaseHandler
	connCodeService *services.ConnectionCodeService
	sessionMgr      *session.SessionManager
}

// Handle 处理 DeleteMapping 命令
func (h *DeleteMappingHandler) Handle(ctx *command.CommandContext) (*command.CommandResponse, error) {
	clientID := h.getClientID(ctx)
	if clientID == 0 {
		return h.errorResponse(ctx, "client not authenticated")
	}

	var req struct {
		MappingID string `json:"mapping_id"`
	}

	if ctx.RequestBody != "" {
		if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
			return h.errorResponse(ctx, fmt.Sprintf("failed to parse request: %v", err))
		}
	}

	if req.MappingID == "" {
		return h.errorResponse(ctx, "mapping_id is required")
	}

	mapping, err := h.connCodeService.GetMapping(req.MappingID)
	if err != nil {
		return h.errorResponse(ctx, fmt.Sprintf("mapping not found: %v", err))
	}

	if mapping.ListenClientID != clientID && mapping.TargetClientID != clientID {
		return h.errorResponse(ctx, "mapping not accessible")
	}

	if err := h.connCodeService.GetPortMappingService().DeletePortMapping(req.MappingID); err != nil {
		return h.errorResponse(ctx, fmt.Sprintf("failed to delete mapping: %v", err))
	}

	return &command.CommandResponse{
		Success:   true,
		Data:      `{"message":"mapping deleted successfully"}`,
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

func (h *DeleteMappingHandler) getClientID(ctx *command.CommandContext) int64 {
	if h.sessionMgr == nil {
		return 0
	}
	controlConn := h.sessionMgr.GetControlConnection(ctx.ConnectionID)
	if controlConn == nil {
		return 0
	}
	return controlConn.ClientID
}

func (h *DeleteMappingHandler) errorResponse(ctx *command.CommandContext, message string) (*command.CommandResponse, error) {
	return &command.CommandResponse{
		Success:   false,
		Error:     message,
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}
