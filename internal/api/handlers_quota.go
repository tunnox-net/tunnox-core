package api

import (
	"net/http"
)

// UpdateQuotaRequest 更新配额请求
type UpdateQuotaRequest struct {
	MaxClientIDs   int   `json:"max_client_ids,omitempty"`
	MaxConnections int   `json:"max_connections,omitempty"`
	BandwidthLimit int64 `json:"bandwidth_limit,omitempty"`
	StorageLimit   int64 `json:"storage_limit,omitempty"`
}

// handleGetUserQuota 获取用户配额
func (s *ManagementAPIServer) handleGetUserQuota(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	// 获取用户信息
	user, err := s.cloudControl.GetUser(userID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	
	// 返回用户配额（从用户信息中获取）
	quota := user.Quota
	
	s.respondJSON(w, http.StatusOK, quota)
}

// handleUpdateUserQuota 更新用户配额
func (s *ManagementAPIServer) handleUpdateUserQuota(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	var req UpdateQuotaRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	// 获取用户信息
	user, err := s.cloudControl.GetUser(userID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	
	// 更新配额字段
	if req.MaxClientIDs > 0 {
		user.Quota.MaxClientIDs = req.MaxClientIDs
	}
	if req.MaxConnections > 0 {
		user.Quota.MaxConnections = req.MaxConnections
	}
	if req.BandwidthLimit >= 0 {
		user.Quota.BandwidthLimit = req.BandwidthLimit
	}
	if req.StorageLimit >= 0 {
		user.Quota.StorageLimit = req.StorageLimit
	}
	
	// 保存用户信息
	if err := s.cloudControl.UpdateUser(user); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	s.respondJSON(w, http.StatusOK, &user.Quota)
}

