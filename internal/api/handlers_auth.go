package api

import (
	"net/http"
	"tunnox-core/internal/cloud/models"
)

// LoginRequest 登录请求
type LoginRequest struct {
	ClientID int64  `json:"client_id"`
	AuthCode string `json:"auth_code"`
	DeviceID string `json:"device_id,omitempty"`
}

// RefreshTokenRequest 刷新token请求
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RevokeTokenRequest 撤销token请求
type RevokeTokenRequest struct {
	Token string `json:"token"`
}

// handleLogin 客户端登录（生成JWT）
func (s *ManagementAPIServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	// 验证必填字段
	if req.ClientID == 0 || req.AuthCode == "" {
		s.respondError(w, http.StatusBadRequest, "client_id and auth_code are required")
		return
	}
	
	// 构造认证请求
	authReq := &models.AuthRequest{
		ClientID:  req.ClientID,
		AuthCode:  req.AuthCode,
		IPAddress: r.RemoteAddr,
	}
	
	// 认证
	authResp, err := s.cloudControl.Authenticate(authReq)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	if !authResp.Success {
		s.respondError(w, http.StatusUnauthorized, authResp.Message)
		return
	}
	
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"token":      authResp.Token,
		"expires_at": authResp.ExpiresAt,
		"client":     authResp.Client,
		"message":    authResp.Message,
	})
}

// handleRefreshToken 刷新Token
func (s *ManagementAPIServer) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	var req RefreshTokenRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	if req.RefreshToken == "" {
		s.respondError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}
	
	// 刷新token
	tokenInfo, err := s.cloudControl.RefreshJWTToken(req.RefreshToken)
	if err != nil {
		s.respondError(w, http.StatusUnauthorized, err.Error())
		return
	}
	
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"token":      tokenInfo.Token,
		"expires_at": tokenInfo.ExpiresAt,
		"message":    "Token refreshed successfully",
	})
}

// handleRevokeToken 撤销Token
func (s *ManagementAPIServer) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	var req RevokeTokenRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	if req.Token == "" {
		s.respondError(w, http.StatusBadRequest, "token is required")
		return
	}
	
	// 撤销token
	if err := s.cloudControl.RevokeJWTToken(req.Token); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	s.respondJSON(w, http.StatusOK, map[string]string{
		"message": "Token revoked successfully",
	})
}

// handleValidateToken 验证Token
func (s *ManagementAPIServer) handleValidateToken(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	
	if token == "" {
		s.respondError(w, http.StatusBadRequest, "token is required")
		return
	}
	
	// 去除 "Bearer " 前缀
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}
	
	// 验证token
	authResp, err := s.cloudControl.ValidateToken(token)
	if err != nil {
		s.respondError(w, http.StatusUnauthorized, err.Error())
		return
	}
	
	if !authResp.Success {
		s.respondError(w, http.StatusUnauthorized, authResp.Message)
		return
	}
	
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"client":     authResp.Client,
		"expires_at": authResp.ExpiresAt,
		"message":    "Token is valid",
	})
}

