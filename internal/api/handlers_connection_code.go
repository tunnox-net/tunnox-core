package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/utils"
)

// ConnectionCodeHandlers 连接码API处理器
type ConnectionCodeHandlers struct {
	connCodeService *services.ConnectionCodeService
}

// NewConnectionCodeHandlers 创建连接码API处理器
func NewConnectionCodeHandlers(connCodeService *services.ConnectionCodeService) *ConnectionCodeHandlers {
	return &ConnectionCodeHandlers{
		connCodeService: connCodeService,
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码管理
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CreateConnectionCodeRequest API请求：创建连接码
type CreateConnectionCodeRequest struct {
	TargetClientID  int64  `json:"target_client_id"` // 生成连接码的客户端
	TargetAddress   string `json:"target_address"`   // 目标地址（必填）
	ActivationTTL   int64  `json:"activation_ttl"`   // 激活有效期（秒，默认600=10分钟）
	MappingDuration int64  `json:"mapping_duration"` // 映射有效期（秒，默认604800=7天）
	Description     string `json:"description"`      // 描述（可选）
}

// CreateConnectionCodeResponse API响应：创建连接码
type CreateConnectionCodeResponse struct {
	Code                string `json:"code"`                  // 连接码
	TargetClientID      int64  `json:"target_client_id"`      // 目标客户端
	TargetAddress       string `json:"target_address"`        // 目标地址
	ActivationExpiresAt string `json:"activation_expires_at"` // 激活截止时间
	MappingDurationSec  int64  `json:"mapping_duration_sec"`  // 映射有效期（秒）
	CreatedAt           string `json:"created_at"`            // 创建时间
}

// HandleCreateConnectionCode 处理创建连接码请求
//
// POST /tunnox/v1/connection-codes
func (h *ConnectionCodeHandlers) HandleCreateConnectionCode(w http.ResponseWriter, r *http.Request) {
	var req CreateConnectionCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Warnf("ConnectionCodeAPI: invalid request body: %v", err)
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// 参数验证
	if req.TargetClientID == 0 {
		respondError(w, http.StatusBadRequest, "target_client_id is required")
		return
	}
	if req.TargetAddress == "" {
		respondError(w, http.StatusBadRequest, "target_address is required")
		return
	}

	// 设置默认值
	activationTTL := time.Duration(req.ActivationTTL) * time.Second
	if activationTTL == 0 {
		activationTTL = 10 * time.Minute
	}

	mappingDuration := time.Duration(req.MappingDuration) * time.Second
	if mappingDuration == 0 {
		mappingDuration = 7 * 24 * time.Hour
	}

	// 创建连接码
	connCode, err := h.connCodeService.CreateConnectionCode(&services.CreateConnectionCodeRequest{
		TargetClientID:  req.TargetClientID,
		TargetAddress:   req.TargetAddress,
		ActivationTTL:   activationTTL,
		MappingDuration: mappingDuration,
		Description:     req.Description,
		CreatedBy:       fmt.Sprintf("api-client-%d", req.TargetClientID),
	})
	if err != nil {
		utils.Errorf("ConnectionCodeAPI: failed to create connection code: %v", err)
		if strings.Contains(err.Error(), "quota exceeded") {
			respondError(w, http.StatusTooManyRequests, err.Error())
		} else {
			respondError(w, http.StatusInternalServerError, "failed to create connection code")
		}
		return
	}

	// 构造响应
	resp := CreateConnectionCodeResponse{
		Code:                connCode.Code,
		TargetClientID:      connCode.TargetClientID,
		TargetAddress:       connCode.TargetAddress,
		ActivationExpiresAt: connCode.ActivationExpiresAt.Format(time.RFC3339),
		MappingDurationSec:  int64(connCode.MappingDuration.Seconds()),
		CreatedAt:           connCode.CreatedAt.Format(time.RFC3339),
	}

	utils.Infof("ConnectionCodeAPI: created connection code %s for client %d",
		connCode.Code, req.TargetClientID)

	respondSuccess(w, http.StatusOK, resp)
}

// ActivateConnectionCodeRequest API请求：激活连接码
type ActivateConnectionCodeRequest struct {
	Code           string `json:"code"`             // 连接码
	ListenClientID int64  `json:"listen_client_id"` // 激活者
	ListenAddress  string `json:"listen_address"`   // 监听地址
}

// ActivateConnectionCodeResponse API响应：激活连接码
type ActivateConnectionCodeResponse struct {
	MappingID      string `json:"mapping_id"`       // 映射ID
	ListenClientID int64  `json:"listen_client_id"` // ListenClient
	TargetClientID int64  `json:"target_client_id"` // TargetClient
	ListenAddress  string `json:"listen_address"`   // 监听地址
	TargetAddress  string `json:"target_address"`   // 目标地址
	ExpiresAt      string `json:"expires_at"`       // 过期时间
	CreatedAt      string `json:"created_at"`       // 创建时间
}

// HandleActivateConnectionCode 处理激活连接码请求
//
// POST /tunnox/v1/connection-codes/:code/activate
func (h *ConnectionCodeHandlers) HandleActivateConnectionCode(w http.ResponseWriter, r *http.Request) {
	// 从URL路径提取code
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		respondError(w, http.StatusBadRequest, "invalid URL path")
		return
	}
	code := pathParts[3]

	var req ActivateConnectionCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Warnf("ConnectionCodeAPI: invalid request body: %v", err)
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Code = code

	// 参数验证
	if req.ListenClientID == 0 {
		respondError(w, http.StatusBadRequest, "listen_client_id is required")
		return
	}
	if req.ListenAddress == "" {
		respondError(w, http.StatusBadRequest, "listen_address is required")
		return
	}

	// 激活连接码
	mapping, err := h.connCodeService.ActivateConnectionCode(&services.ActivateConnectionCodeRequest{
		Code:           req.Code,
		ListenClientID: req.ListenClientID,
		ListenAddress:  req.ListenAddress,
	})
	if err != nil {
		utils.Errorf("ConnectionCodeAPI: failed to activate connection code %s: %v", req.Code, err)
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "expired") {
			respondError(w, http.StatusNotFound, err.Error())
		} else if strings.Contains(err.Error(), "quota exceeded") {
			respondError(w, http.StatusTooManyRequests, err.Error())
		} else if strings.Contains(err.Error(), "already been used") || strings.Contains(err.Error(), "revoked") {
			respondError(w, http.StatusConflict, err.Error())
		} else {
			respondError(w, http.StatusInternalServerError, "failed to activate connection code")
		}
		return
	}

	// 构造响应
	var expiresAtStr string
	if mapping.ExpiresAt != nil {
		expiresAtStr = mapping.ExpiresAt.Format(time.RFC3339)
	}

	resp := ActivateConnectionCodeResponse{
		MappingID:      mapping.ID,
		ListenClientID: mapping.ListenClientID,
		TargetClientID: mapping.TargetClientID,
		ListenAddress:  mapping.ListenAddress,
		TargetAddress:  mapping.TargetAddress,
		ExpiresAt:      expiresAtStr,
		CreatedAt:      mapping.CreatedAt.Format(time.RFC3339),
	}

	utils.Infof("ConnectionCodeAPI: activated code %s, created mapping %s",
		req.Code, mapping.ID)

	respondSuccess(w, http.StatusOK, resp)
}

// HandleRevokeConnectionCode 处理撤销连接码请求
//
// DELETE /tunnox/v1/connection-codes/:code
func (h *ConnectionCodeHandlers) HandleRevokeConnectionCode(w http.ResponseWriter, r *http.Request) {
	// 从URL路径提取code
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		respondError(w, http.StatusBadRequest, "invalid URL path")
		return
	}
	code := pathParts[3]

	// 撤销连接码
	if err := h.connCodeService.RevokeConnectionCode(code, "api-admin"); err != nil {
		utils.Errorf("ConnectionCodeAPI: failed to revoke connection code %s: %v", code, err)
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "connection code not found")
		} else if strings.Contains(err.Error(), "already been used") {
			respondError(w, http.StatusConflict, err.Error())
		} else {
			respondError(w, http.StatusInternalServerError, "failed to revoke connection code")
		}
		return
	}

	utils.Infof("ConnectionCodeAPI: revoked connection code %s", code)

	respondSuccess(w, http.StatusOK, map[string]string{
		"message": "connection code revoked successfully",
	})
}

// ConnectionCodeListItem 连接码列表项
type ConnectionCodeListItem struct {
	Code                string  `json:"code"`
	TargetClientID      int64   `json:"target_client_id"`
	TargetAddress       string  `json:"target_address"`
	ActivationExpiresAt string  `json:"activation_expires_at"`
	IsActivated         bool    `json:"is_activated"`
	MappingID           *string `json:"mapping_id,omitempty"`
	CreatedAt           string  `json:"created_at"`
	Description         string  `json:"description,omitempty"`
}

// HandleListConnectionCodes 处理列出连接码请求
//
// GET /tunnox/v1/connection-codes?target_client_id=xxx
// GET /tunnox/v1/connection-codes?client_id=xxx (向后兼容)
func (h *ConnectionCodeHandlers) HandleListConnectionCodes(w http.ResponseWriter, r *http.Request) {
	// ✅ 优先使用 target_client_id，如果没有则使用 client_id（向后兼容）
	targetClientIDStr := r.URL.Query().Get("target_client_id")
	if targetClientIDStr == "" {
		targetClientIDStr = r.URL.Query().Get("client_id")
	}
	if targetClientIDStr == "" {
		respondError(w, http.StatusBadRequest, "target_client_id or client_id is required")
		return
	}

	targetClientID, err := strconv.ParseInt(targetClientIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid target_client_id")
		return
	}

	// 列出连接码
	codes, err := h.connCodeService.ListConnectionCodesByTargetClient(targetClientID)
	if err != nil {
		utils.Errorf("ConnectionCodeAPI: failed to list connection codes for client %d: %v",
			targetClientID, err)
		respondError(w, http.StatusInternalServerError, "failed to list connection codes")
		return
	}

	// 构造响应
	items := make([]ConnectionCodeListItem, 0, len(codes))
	for _, code := range codes {
		item := ConnectionCodeListItem{
			Code:                code.Code,
			TargetClientID:      code.TargetClientID,
			TargetAddress:       code.TargetAddress,
			ActivationExpiresAt: code.ActivationExpiresAt.Format(time.RFC3339),
			IsActivated:         code.IsActivated,
			MappingID:           code.MappingID,
			CreatedAt:           code.CreatedAt.Format(time.RFC3339),
			Description:         code.Description,
		}
		items = append(items, item)
	}

	respondSuccess(w, http.StatusOK, map[string]interface{}{
		"total": len(items),
		"codes": items,
	})
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 映射管理
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// MappingListItem 映射列表项
type MappingListItem struct {
	ID             string `json:"id"`
	ListenClientID int64  `json:"listen_client_id"`
	TargetClientID int64  `json:"target_client_id"`
	ListenAddress  string `json:"listen_address"`
	TargetAddress  string `json:"target_address"`
	ExpiresAt      string `json:"expires_at"`
	CreatedAt      string `json:"created_at"`
	UsageCount     int64  `json:"usage_count"`
	BytesSent      int64  `json:"bytes_sent"`
	BytesReceived  int64  `json:"bytes_received"`
	Description    string `json:"description,omitempty"`
}

// HandleListMappings 处理列出映射请求
//
// GET /tunnox/v1/mappings?client_id=xxx&direction=outbound|inbound
func (h *ConnectionCodeHandlers) HandleListMappings(w http.ResponseWriter, r *http.Request) {
	// 从查询参数获取client_id和direction
	clientIDStr := r.URL.Query().Get("client_id")
	if clientIDStr == "" {
		respondError(w, http.StatusBadRequest, "client_id is required")
		return
	}

	clientID, err := strconv.ParseInt(clientIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid client_id")
		return
	}

	direction := r.URL.Query().Get("direction")
	if direction == "" {
		direction = "outbound" // 默认查询出站映射
	}

	// 列出映射
	var mappings []*models.PortMapping
	switch direction {
	case "outbound":
		mappings, err = h.connCodeService.ListOutboundMappings(clientID)
	case "inbound":
		mappings, err = h.connCodeService.ListInboundMappings(clientID)
	default:
		respondError(w, http.StatusBadRequest, "invalid direction, must be 'outbound' or 'inbound'")
		return
	}

	if err != nil {
		utils.Errorf("ConnectionCodeAPI: failed to list %s mappings for client %d: %v",
			direction, clientID, err)
		respondError(w, http.StatusInternalServerError, "failed to list mappings")
		return
	}

	// 构造响应
	items := make([]MappingListItem, 0, len(mappings))
	for _, mapping := range mappings {
		expiresAtStr := ""
		if mapping.ExpiresAt != nil {
			expiresAtStr = mapping.ExpiresAt.Format(time.RFC3339)
		}
		item := MappingListItem{
			ID:             mapping.ID,
			ListenClientID: mapping.ListenClientID,
			TargetClientID: mapping.TargetClientID,
			ListenAddress:  mapping.ListenAddress,
			TargetAddress:  mapping.TargetAddress,
			ExpiresAt:      expiresAtStr,
			CreatedAt:      mapping.CreatedAt.Format(time.RFC3339),
			UsageCount:     0, // PortMapping 不使用 UsageCount，使用 LastActive 记录最后使用时间
			BytesSent:      mapping.TrafficStats.BytesSent,
			BytesReceived:  mapping.TrafficStats.BytesReceived,
			Description:    mapping.Description,
		}
		items = append(items, item)
	}

	respondSuccess(w, http.StatusOK, map[string]interface{}{
		"total":     len(items),
		"direction": direction,
		"mappings":  items,
	})
}

// HandleRevokeMapping 处理撤销映射请求
//
// DELETE /tunnox/v1/mappings/:id?client_id=xxx
func (h *ConnectionCodeHandlers) HandleRevokeMapping(w http.ResponseWriter, r *http.Request) {
	// 从URL路径提取mapping ID
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		respondError(w, http.StatusBadRequest, "invalid URL path")
		return
	}
	mappingID := pathParts[3]

	// 从查询参数获取client_id（用于权限检查）
	clientIDStr := r.URL.Query().Get("client_id")
	if clientIDStr == "" {
		respondError(w, http.StatusBadRequest, "client_id is required")
		return
	}

	clientID, err := strconv.ParseInt(clientIDStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid client_id")
		return
	}

	// 撤销映射
	if err := h.connCodeService.RevokeMapping(mappingID, clientID, "api-admin"); err != nil {
		utils.Errorf("ConnectionCodeAPI: failed to revoke mapping %s: %v", mappingID, err)
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "mapping not found")
		} else if strings.Contains(err.Error(), "not authorized") {
			respondError(w, http.StatusForbidden, err.Error())
		} else {
			respondError(w, http.StatusInternalServerError, "failed to revoke mapping")
		}
		return
	}

	utils.Infof("ConnectionCodeAPI: revoked mapping %s by client %d", mappingID, clientID)

	respondSuccess(w, http.StatusOK, map[string]string{
		"message": "mapping revoked successfully",
	})
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 辅助方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// respondSuccess 返回成功响应
func respondSuccess(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    data,
	})
}

// respondError 返回错误响应
func respondError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   message,
	})
}
