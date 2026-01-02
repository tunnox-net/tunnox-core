package management

import (
	"net/http"

	"tunnox-core/internal/cloud/models"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
)

// handleCreateUser 创建用户
func (m *ManagementModule) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	user, err := m.cloudControl.CreateUser(req.Username, req.Email)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to create user: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusCreated, user)
}

// handleGetUser 获取用户
func (m *ManagementModule) handleGetUser(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	user, err := m.cloudControl.GetUser(userID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, user)
}

// handleUpdateUser 更新用户
func (m *ManagementModule) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		Username string            `json:"username,omitempty"`
		Email    string            `json:"email,omitempty"`
		Status   models.UserStatus `json:"status,omitempty"`
		Plan     models.UserPlan   `json:"plan,omitempty"`
		Quota    *models.UserQuota `json:"quota,omitempty"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 获取现有用户
	user, err := m.cloudControl.GetUser(userID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// 更新字段
	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Status != "" {
		user.Status = req.Status
	}
	if req.Plan != "" {
		user.Plan = req.Plan
	}
	if req.Quota != nil {
		user.Quota = *req.Quota
	}

	// 保存更新
	if err := m.cloudControl.UpdateUser(user); err != nil {
		corelog.Errorf("ManagementModule: failed to update user %s: %v", userID, err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, user)
}

// handleDeleteUser 删除用户
func (m *ManagementModule) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	if err := m.cloudControl.DeleteUser(userID); err != nil {
		corelog.Errorf("ManagementModule: failed to delete user %s: %v", userID, err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, httpservice.MessageResponse{Message: "user deleted"})
}

// handleListUsers 列出用户
func (m *ManagementModule) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 获取查询参数
	userType := models.UserType(r.URL.Query().Get("type"))

	users, err := m.cloudControl.ListUsers(userType)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to list users: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, users)
}

// handleListUserClients 列出用户的客户端
func (m *ManagementModule) handleListUserClients(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	clients, err := m.cloudControl.ListUserClients(userID)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to list user clients: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 包装成对象返回，符合 platform 期望的格式
	respondJSONTyped(w, http.StatusOK, map[string]interface{}{
		"clients": clients,
	})
}

// handleListUserMappings 列出用户的映射
func (m *ManagementModule) handleListUserMappings(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	mappings, err := m.cloudControl.GetUserPortMappings(userID)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to list user mappings: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 包装成对象返回，符合 platform 期望的格式
	respondJSONTyped(w, http.StatusOK, map[string]interface{}{
		"mappings": mappings,
	})
}
