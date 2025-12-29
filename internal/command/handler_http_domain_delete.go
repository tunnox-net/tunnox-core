package command

import (
	"encoding/json"
	"fmt"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP 域名映射删除相关 Handler
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// HTTPDomainDeleteHandler 删除 HTTP 域名映射处理器
type HTTPDomainDeleteHandler struct {
	*BaseHandler
	deleter HTTPDomainDeleter
}

// NewHTTPDomainDeleteHandler 创建处理器
func NewHTTPDomainDeleteHandler(deleter HTTPDomainDeleter) *HTTPDomainDeleteHandler {
	return &HTTPDomainDeleteHandler{
		BaseHandler: NewBaseHandler(
			packet.HTTPDomainDelete,
			CategoryMapping,
			DirectionDuplex,
			"http_domain_delete",
			"删除 HTTP 域名映射",
		),
		deleter: deleter,
	}
}

func (h *HTTPDomainDeleteHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	corelog.Infof("HTTPDomainDeleteHandler: handling request from connection %s, clientID=%d", ctx.ConnectionID, ctx.ClientID)

	var req packet.HTTPDomainDeleteRequest
	if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
		resp := packet.HTTPDomainDeleteResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid request: %v", err),
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	if req.MappingID == "" {
		resp := packet.HTTPDomainDeleteResponse{
			Success: false,
			Error:   "mapping_id is required",
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	if h.deleter == nil {
		resp := packet.HTTPDomainDeleteResponse{
			Success: false,
			Error:   "domain deleter not configured",
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	if err := h.deleter.DeleteHTTPDomainMapping(ctx.ClientID, req.MappingID); err != nil {
		resp := packet.HTTPDomainDeleteResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to delete mapping: %v", err),
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	resp := packet.HTTPDomainDeleteResponse{
		Success: true,
	}

	data, _ := json.Marshal(resp)
	corelog.Infof("HTTPDomainDeleteHandler: deleted mapping %s", req.MappingID)
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// SetDeleter 设置删除器
func (h *HTTPDomainDeleteHandler) SetDeleter(deleter HTTPDomainDeleter) {
	h.deleter = deleter
}
