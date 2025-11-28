package server

import (
	"encoding/json"
	"fmt"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/command"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
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
		return fmt.Errorf("command registry is nil")
	}

	// 注册 ListMappings 命令
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
		return fmt.Errorf("failed to register list mappings handler: %w", err)
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

	utils.Infof("ListMappingsHandler: client %d requesting mappings", clientID)

	// 解析请求参数
	var req struct {
		Direction string `json:"direction"` // outbound | inbound
		Type      string `json:"type"`      // 映射类型过滤
		Status    string `json:"status"`    // 状态过滤
	}

	if ctx.RequestBody != "" {
		if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
			utils.Warnf("ListMappingsHandler: failed to parse request body: %v", err)
		}
	}

	utils.Infof("ListMappingsHandler: request params - direction=%s, type=%s, status=%s", req.Direction, req.Type, req.Status)

	// 获取映射列表
	var mappings []*models.PortMapping
	var err error

	switch req.Direction {
	case "outbound":
		mappings, err = h.connCodeService.ListOutboundMappings(clientID)
		utils.Infof("ListMappingsHandler: ListOutboundMappings returned %d mappings", len(mappings))
	case "inbound":
		mappings, err = h.connCodeService.ListInboundMappings(clientID)
		utils.Infof("ListMappingsHandler: ListInboundMappings returned %d mappings", len(mappings))
	default:
		// 获取所有映射
		outboundMappings, err1 := h.connCodeService.ListOutboundMappings(clientID)
		inboundMappings, err2 := h.connCodeService.ListInboundMappings(clientID)
		utils.Infof("ListMappingsHandler: ListOutboundMappings returned %d mappings, ListInboundMappings returned %d mappings", len(outboundMappings), len(inboundMappings))
		if err1 != nil {
			err = err1
		} else if err2 != nil {
			err = err2
		} else {
			// 合并并去重
			mappingMap := make(map[string]*models.PortMapping)
			for _, m := range outboundMappings {
				mappingMap[m.ID] = m
				utils.Debugf("ListMappingsHandler: added outbound mapping %s (ListenClientID=%d, TargetClientID=%d)", m.ID, m.ListenClientID, m.TargetClientID)
			}
			for _, m := range inboundMappings {
				mappingMap[m.ID] = m
				utils.Debugf("ListMappingsHandler: added inbound mapping %s (ListenClientID=%d, TargetClientID=%d)", m.ID, m.ListenClientID, m.TargetClientID)
			}
			mappings = make([]*models.PortMapping, 0, len(mappingMap))
			for _, m := range mappingMap {
				mappings = append(mappings, m)
			}
			utils.Infof("ListMappingsHandler: merged %d unique mappings", len(mappings))
		}
	}

	// ✅ 如果索引为空，记录警告（索引应该在创建映射时自动维护）
	// 注意：不从全局列表加载所有数据再过滤，避免性能问题
	// 如果存储层支持按字段查询，可以在存储层实现；否则应该确保索引正确维护
	if len(mappings) == 0 && err == nil {
		utils.Warnf("ListMappingsHandler: no mappings found from index for client %d. Index may be empty or not properly maintained.", clientID)
	}

	if err != nil {
		utils.Errorf("ListMappingsHandler: failed to list mappings for client %d: %v", clientID, err)
		return h.errorResponse(ctx, fmt.Sprintf("failed to list mappings: %v", err))
	}

	// 转换为响应格式
	mappingItems := make([]map[string]interface{}, 0, len(mappings))
	for _, m := range mappings {
		// 状态过滤
		if req.Status != "" && string(m.Status) != req.Status {
			utils.Debugf("ListMappingsHandler: filtering out mapping %s (status mismatch: %s != %s)", m.ID, m.Status, req.Status)
			continue
		}

		// 类型过滤
		if req.Type != "" && string(m.Protocol) != req.Type {
			utils.Debugf("ListMappingsHandler: filtering out mapping %s (type mismatch: %s != %s)", m.ID, m.Protocol, req.Type)
			continue
		}

		// 确定映射类型（outbound 或 inbound）
		mappingType := "outbound"
		if m.TargetClientID == clientID && m.ListenClientID != clientID {
			mappingType = "inbound"
		}

		expiresAtStr := ""
		if m.ExpiresAt != nil {
			expiresAtStr = m.ExpiresAt.Format(time.RFC3339)
		}

		item := map[string]interface{}{
			"mapping_id":     m.ID,
			"type":           mappingType,
			"target_address": m.TargetAddress,
			"listen_address": m.ListenAddress,
			"status":         string(m.Status),
			"expires_at":     expiresAtStr,
			"created_at":     m.CreatedAt.Format(time.RFC3339),
			"bytes_sent":     m.TrafficStats.BytesSent,
			"bytes_received": m.TrafficStats.BytesReceived,
		}

		mappingItems = append(mappingItems, item)
		utils.Debugf("ListMappingsHandler: added mapping %s (type=%s, status=%s)", m.ID, mappingType, m.Status)
	}

	utils.Infof("ListMappingsHandler: returning %d mappings (after filtering)", len(mappingItems))

	resp := map[string]interface{}{
		"mappings": mappingItems,
		"total":    len(mappingItems),
	}

	respBody, _ := json.Marshal(resp)
	utils.Debugf("ListMappingsHandler: response body length=%d", len(respBody))
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
