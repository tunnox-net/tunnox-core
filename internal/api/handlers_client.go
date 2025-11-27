package api

import (
	"fmt"
	"net/http"
	"tunnox-core/internal/cloud/models"
)

// CreateClientRequest 创建客户端请求
type CreateClientRequest struct {
	UserID     string `json:"user_id"`
	ClientName string `json:"client_name"`
	ClientDesc string `json:"client_desc,omitempty"`
}

// UpdateClientRequest 更新客户端请求
type UpdateClientRequest struct {
	ClientName string `json:"client_name,omitempty"`
	Status     string `json:"status,omitempty"`
}

// ClaimClientRequest 认领匿名客户端请求
type ClaimClientRequest struct {
	AnonymousClientID int64  `json:"anonymous_client_id"`
	UserID            string `json:"user_id"`
	NewClientName     string `json:"new_client_name"`
}

// handleCreateClient 创建托管客户端
func (s *ManagementAPIServer) handleCreateClient(w http.ResponseWriter, r *http.Request) {
	var req CreateClientRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 验证必填字段
	if req.UserID == "" || req.ClientName == "" {
		s.respondError(w, http.StatusBadRequest, "user_id and client_name are required")
		return
	}

	// 创建客户端
	client, err := s.cloudControl.CreateClient(req.UserID, req.ClientName)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusCreated, client)
}

// handleGetClient 获取客户端信息
func (s *ManagementAPIServer) handleGetClient(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	client, err := s.cloudControl.GetClient(clientID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, client)
}

// handleUpdateClient 更新客户端信息
func (s *ManagementAPIServer) handleUpdateClient(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdateClientRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 获取现有客户端
	client, err := s.cloudControl.GetClient(clientID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// 更新字段
	if req.ClientName != "" {
		client.Name = req.ClientName
	}
	if req.Status != "" {
		client.Status = models.ClientStatus(req.Status)
	}

	// 保存更新
	if err := s.cloudControl.UpdateClient(client); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, client)
}

// handleDeleteClient 删除客户端
func (s *ManagementAPIServer) handleDeleteClient(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.cloudControl.DeleteClient(clientID); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDisconnectClient 强制下线客户端
func (s *ManagementAPIServer) handleDisconnectClient(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 发送踢下线命令
	s.kickClient(clientID, "Disconnected by administrator", "ADMIN_DISCONNECT")

	// 更新客户端状态为离线
	if err := s.cloudControl.UpdateClientStatus(clientID, models.ClientStatusOffline, ""); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"message": "Client disconnected successfully",
	})
}

// handleClaimClient 认领匿名客户端
func (s *ManagementAPIServer) handleClaimClient(w http.ResponseWriter, r *http.Request) {
	var req ClaimClientRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 验证必填字段
	if req.AnonymousClientID == 0 || req.UserID == "" {
		s.respondError(w, http.StatusBadRequest, "anonymous_client_id and user_id are required")
		return
	}

	// 客户端名称
	clientName := req.NewClientName
	if clientName == "" {
		clientName = fmt.Sprintf("Claimed-%d", req.AnonymousClientID)
	}

	// 使用事务处理认领流程
	result, err := s.claimClientWithTransaction(req.AnonymousClientID, req.UserID, clientName)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, result)
}

// handleListClientMappings 列出客户端的端口映射
func (s *ManagementAPIServer) handleListClientMappings(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	mappings, err := s.cloudControl.GetClientPortMappings(clientID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"mappings": mappings,
		"total":    len(mappings),
	})
}
