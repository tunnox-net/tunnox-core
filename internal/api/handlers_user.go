package api

import (
	"net/http"
	"tunnox-core/internal/cloud/models"
)

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash,omitempty"`
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Email  string `json:"email,omitempty"`
	Status string `json:"status,omitempty"`
}

// handleCreateUser 创建用户
func (s *ManagementAPIServer) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 验证必填字段
	if req.Username == "" || req.Email == "" {
		s.respondError(w, http.StatusBadRequest, "username and email are required")
		return
	}

	// 创建用户
	user, err := s.cloudControl.CreateUser(req.Username, req.Email)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusCreated, user)
}

// handleGetUser 获取用户信息
func (s *ManagementAPIServer) handleGetUser(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := s.cloudControl.GetUser(userID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, user)
}

// handleUpdateUser 更新用户信息
func (s *ManagementAPIServer) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdateUserRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 获取现有用户
	user, err := s.cloudControl.GetUser(userID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// 更新字段
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Status != "" {
		user.Status = models.UserStatus(req.Status)
	}

	// 保存更新
	if err := s.cloudControl.UpdateUser(user); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, user)
}

// handleDeleteUser 删除用户
func (s *ManagementAPIServer) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.cloudControl.DeleteUser(userID); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListUsers 列出用户
func (s *ManagementAPIServer) handleListUsers(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	userType := r.URL.Query().Get("type")
	
	// 根据类型获取用户列表
	var users []*models.User
	var err error
	
	if userType != "" {
		users, err = s.cloudControl.ListUsers(models.UserType(userType))
	} else {
		users, err = s.cloudControl.ListUsers("")
	}

	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := UserListResponse{
		Users: users,
		Total: len(users),
	}
	s.respondJSON(w, http.StatusOK, response)
}

// handleListUserClients 列出用户的客户端
func (s *ManagementAPIServer) handleListUserClients(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	clients, err := s.cloudControl.ListUserClients(userID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := ClientListResponse{
		Clients: clients,
		Total:   len(clients),
	}
	s.respondJSON(w, http.StatusOK, response)
}

// handleListUserMappings 列出用户的端口映射
func (s *ManagementAPIServer) handleListUserMappings(w http.ResponseWriter, r *http.Request) {
	userID, err := getStringPathVar(r, "user_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	mappings, err := s.cloudControl.GetUserPortMappings(userID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := MappingListResponse{
		Mappings: mappings,
		Total:    len(mappings),
	}
	s.respondJSON(w, http.StatusOK, response)
}

