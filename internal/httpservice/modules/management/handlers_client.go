package management

import (
	"net/http"

	"tunnox-core/internal/cloud/models"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
)

// handleListAllClients 列出所有客户端
func (m *ManagementModule) handleListAllClients(w http.ResponseWriter, r *http.Request) {
	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 列出所有类型的客户端
	clients, err := m.cloudControl.ListClients("", "")
	if err != nil {
		corelog.Errorf("ManagementModule: failed to list all clients: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, clients)
}

// handleCreateClient 创建客户端
func (m *ManagementModule) handleCreateClient(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Name   string `json:"name"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	client, err := m.cloudControl.CreateClient(req.UserID, req.Name)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to create client: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusCreated, client)
}

// handleGetClient 获取客户端
func (m *ManagementModule) handleGetClient(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	client, err := m.cloudControl.GetClient(clientID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, client)
}

// handleUpdateClient 更新客户端
func (m *ManagementModule) handleUpdateClient(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		Name   string              `json:"name,omitempty"`
		Status models.ClientStatus `json:"status,omitempty"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 获取现有客户端
	client, err := m.cloudControl.GetClient(clientID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// 更新字段
	if req.Name != "" {
		client.Name = req.Name
	}
	if req.Status != "" {
		client.Status = req.Status
	}

	// 保存更新
	if err := m.cloudControl.UpdateClient(client); err != nil {
		corelog.Errorf("ManagementModule: failed to update client %d: %v", clientID, err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, client)
}

// handleDeleteClient 删除客户端
func (m *ManagementModule) handleDeleteClient(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	if err := m.cloudControl.DeleteClient(clientID); err != nil {
		corelog.Errorf("ManagementModule: failed to delete client %d: %v", clientID, err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, httpservice.MessageResponse{Message: "client deleted"})
}

// handleDisconnectClient 断开客户端连接
func (m *ManagementModule) handleDisconnectClient(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 更新客户端状态为离线
	if err := m.cloudControl.UpdateClientStatus(clientID, models.ClientStatusOffline, ""); err != nil {
		corelog.Errorf("ManagementModule: failed to disconnect client %d: %v", clientID, err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, httpservice.MessageResponse{Message: "client disconnected"})
}

// handleListClientMappings 列出客户端的映射
func (m *ManagementModule) handleListClientMappings(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	mappings, err := m.cloudControl.GetClientPortMappings(clientID)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to list client mappings: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, mappings)
}

// handleGetClientQuota 获取客户端配额
// GET /tunnox/v1/clients/{client_id}/quota
func (m *ManagementModule) handleGetClientQuota(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 获取客户端信息
	client, err := m.cloudControl.GetClient(clientID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// 如果客户端有关联用户，获取用户配额
	var quota *models.UserQuota
	if client.UserID != "" {
		user, err := m.cloudControl.GetUser(client.UserID)
		if err == nil {
			quota = &user.Quota
		}
	}

	// 如果没有用户或获取失败，返回默认配额
	if quota == nil {
		quota = &models.UserQuota{
			MaxClientIDs:   10,
			MaxConnections: 100,
			BandwidthLimit: 0, // 无限制
			StorageLimit:   0,
		}
	}

	respondJSONTyped(w, http.StatusOK, quota)
}
