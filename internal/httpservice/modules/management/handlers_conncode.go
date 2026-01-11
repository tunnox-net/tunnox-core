package management

import (
	"net/http"
	"strconv"
	"time"

	"tunnox-core/internal/cloud/services/conncode"

	"github.com/gorilla/mux"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码管理 HTTP Handlers
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CreateConnectionCodeRequest HTTP 请求结构
type CreateConnectionCodeRequest struct {
	TargetClientID  int64  `json:"target_client_id"`
	TargetAddress   string `json:"target_address"`
	ActivationMins  int    `json:"activation_mins,omitempty"`  // 激活有效期(分钟)，默认10
	MappingDuration int    `json:"mapping_duration,omitempty"` // 映射有效期(小时)，默认168(7天)
	Description     string `json:"description,omitempty"`
}

// ConnectionCodeResponse HTTP 响应结构
type ConnectionCodeResponse struct {
	ID                  string    `json:"id"`
	Code                string    `json:"code"`
	TargetClientID      int64     `json:"target_client_id"`
	TargetAddress       string    `json:"target_address"`
	IsActivated         bool      `json:"is_activated"`
	IsRevoked           bool      `json:"is_revoked"`
	ActivationExpiresAt time.Time `json:"activation_expires_at"`
	CreatedAt           time.Time `json:"created_at"`
	Description         string    `json:"description,omitempty"`
	MappingID           string    `json:"mapping_id,omitempty"`
	ActivatedBy         int64     `json:"activated_by,omitempty"`
	ActivatedAt         time.Time `json:"activated_at,omitempty"`
}

// ActivateConnectionCodeRequest 激活连接码请求
type ActivateConnectionCodeRequest struct {
	Code           string `json:"code"`
	ListenClientID int64  `json:"listen_client_id"`
	ListenAddress  string `json:"listen_address"` // 如 0.0.0.0:9999
}

// ActivateConnectionCodeResponse 激活连接码响应
type ActivateConnectionCodeResponse struct {
	MappingID     string `json:"mapping_id"`
	TargetAddress string `json:"target_address"`
	ListenAddress string `json:"listen_address"`
	ExpiresAt     string `json:"expires_at"`
}

// handleCreateConnectionCode 创建连接码
// POST /tunnox/connection-codes
func (m *ManagementModule) handleCreateConnectionCode(w http.ResponseWriter, r *http.Request) {
	var req CreateConnectionCodeRequest
	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 参数验证
	if req.TargetClientID == 0 {
		m.respondError(w, http.StatusBadRequest, "target_client_id is required")
		return
	}
	if req.TargetAddress == "" {
		m.respondError(w, http.StatusBadRequest, "target_address is required")
		return
	}

	// 设置默认值
	activationTTL := 10 * time.Minute
	if req.ActivationMins > 0 {
		activationTTL = time.Duration(req.ActivationMins) * time.Minute
	}

	mappingDuration := 7 * 24 * time.Hour // 默认7天
	if req.MappingDuration > 0 {
		mappingDuration = time.Duration(req.MappingDuration) * time.Hour
	}

	// 调用服务创建连接码
	createReq := &conncode.CreateRequest{
		TargetClientID:  req.TargetClientID,
		TargetAddress:   req.TargetAddress,
		ActivationTTL:   activationTTL,
		MappingDuration: mappingDuration,
		Description:     req.Description,
		CreatedBy:       "management-api",
	}

	result, err := m.connCodeService.CreateConnectionCode(createReq)
	if err != nil {
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 转换为响应格式
	resp := ConnectionCodeResponse{
		ID:                  result.ID,
		Code:                result.Code,
		TargetClientID:      result.TargetClientID,
		TargetAddress:       result.TargetAddress,
		IsActivated:         result.IsActivated,
		IsRevoked:           result.IsRevoked,
		ActivationExpiresAt: result.ActivationExpiresAt,
		CreatedAt:           result.CreatedAt,
		Description:         result.Description,
	}

	respondJSONTyped(w, http.StatusCreated, resp)
}

// handleListConnectionCodes 列出连接码
// GET /tunnox/connection-codes?target_client_id=xxx
func (m *ManagementModule) handleListConnectionCodes(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	targetClientIDStr := r.URL.Query().Get("target_client_id")
	if targetClientIDStr == "" {
		m.respondError(w, http.StatusBadRequest, "target_client_id query parameter is required")
		return
	}

	targetClientID, err := strconv.ParseInt(targetClientIDStr, 10, 64)
	if err != nil {
		m.respondError(w, http.StatusBadRequest, "invalid target_client_id")
		return
	}

	// 调用服务获取连接码列表
	codes, err := m.connCodeService.ListConnectionCodesByTargetClient(targetClientID)
	if err != nil {
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 转换为响应格式
	respCodes := make([]ConnectionCodeResponse, 0, len(codes))
	for _, code := range codes {
		resp := ConnectionCodeResponse{
			ID:                  code.ID,
			Code:                code.Code,
			TargetClientID:      code.TargetClientID,
			TargetAddress:       code.TargetAddress,
			IsActivated:         code.IsActivated,
			IsRevoked:           code.IsRevoked,
			ActivationExpiresAt: code.ActivationExpiresAt,
			CreatedAt:           code.CreatedAt,
			Description:         code.Description,
		}
		if code.IsActivated {
			if code.MappingID != nil {
				resp.MappingID = *code.MappingID
			}
			if code.ActivatedBy != nil {
				resp.ActivatedBy = *code.ActivatedBy
			}
			if code.ActivatedAt != nil {
				resp.ActivatedAt = *code.ActivatedAt
			}
		}
		respCodes = append(respCodes, resp)
	}

	respondJSONTyped(w, http.StatusOK, map[string]interface{}{
		"codes": respCodes,
		"total": len(respCodes),
	})
}

// handleGetConnectionCode 获取连接码详情
// GET /tunnox/connection-codes/{code}
func (m *ManagementModule) handleGetConnectionCode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	code := vars["code"]
	if code == "" {
		m.respondError(w, http.StatusBadRequest, "code is required")
		return
	}

	// 调用服务获取连接码
	result, err := m.connCodeService.GetConnectionCode(code)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// 转换为响应格式
	resp := ConnectionCodeResponse{
		ID:                  result.ID,
		Code:                result.Code,
		TargetClientID:      result.TargetClientID,
		TargetAddress:       result.TargetAddress,
		IsActivated:         result.IsActivated,
		IsRevoked:           result.IsRevoked,
		ActivationExpiresAt: result.ActivationExpiresAt,
		CreatedAt:           result.CreatedAt,
		Description:         result.Description,
	}
	if result.IsActivated {
		if result.MappingID != nil {
			resp.MappingID = *result.MappingID
		}
		if result.ActivatedBy != nil {
			resp.ActivatedBy = *result.ActivatedBy
		}
		if result.ActivatedAt != nil {
			resp.ActivatedAt = *result.ActivatedAt
		}
	}

	respondJSONTyped(w, http.StatusOK, resp)
}

// handleActivateConnectionCode 激活连接码
// POST /tunnox/connection-codes/activate
func (m *ManagementModule) handleActivateConnectionCode(w http.ResponseWriter, r *http.Request) {
	var req ActivateConnectionCodeRequest
	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 参数验证
	if req.Code == "" {
		m.respondError(w, http.StatusBadRequest, "code is required")
		return
	}
	if req.ListenClientID == 0 {
		m.respondError(w, http.StatusBadRequest, "listen_client_id is required")
		return
	}
	if req.ListenAddress == "" {
		m.respondError(w, http.StatusBadRequest, "listen_address is required")
		return
	}

	// 调用服务激活连接码
	activateReq := &conncode.ActivateRequest{
		Code:           req.Code,
		ListenClientID: req.ListenClientID,
		ListenAddress:  req.ListenAddress,
	}

	mapping, err := m.connCodeService.ActivateConnectionCode(activateReq)
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 转换为响应格式
	expiresAt := ""
	if mapping.ExpiresAt != nil {
		expiresAt = mapping.ExpiresAt.Format(time.RFC3339)
	}

	resp := ActivateConnectionCodeResponse{
		MappingID:     mapping.ID,
		TargetAddress: mapping.TargetAddress,
		ListenAddress: mapping.ListenAddress,
		ExpiresAt:     expiresAt,
	}

	respondJSONTyped(w, http.StatusOK, resp)
}

// handleRevokeConnectionCode 撤销连接码
// DELETE /tunnox/connection-codes/{code}
func (m *ManagementModule) handleRevokeConnectionCode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	code := vars["code"]
	if code == "" {
		m.respondError(w, http.StatusBadRequest, "code is required")
		return
	}

	// 调用服务撤销连接码
	err := m.connCodeService.RevokeConnectionCode(code, "management-api")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
