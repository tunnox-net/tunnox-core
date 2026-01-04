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

	// 填充 IP 地区信息（GeoIP 解析）
	enrichClientsWithIPRegion(clients)

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

// handleBindClient 绑定客户端到用户
// POST /tunnox/v1/clients/{client_id}/bind
// 请求体: { "user_id": "xxx", "secret_key": "xxx", "name": "可选名称" }
func (m *ManagementModule) handleBindClient(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		UserID    string `json:"user_id"`
		SecretKey string `json:"secret_key"`
		Name      string `json:"name"` // 可选的客户端名称
	}

	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.UserID == "" {
		m.respondError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	if req.SecretKey == "" {
		m.respondError(w, http.StatusBadRequest, "secret_key is required")
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 1. 获取客户端
	client, err := m.cloudControl.GetClient(clientID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, "client not found")
		return
	}

	// 2. 验证密钥
	if client.SecretKey != req.SecretKey {
		corelog.Warnf("ManagementModule: bind client %d failed: invalid secret_key", clientID)
		m.respondError(w, http.StatusUnauthorized, "invalid secret_key")
		return
	}

	// 3. 检查是否已绑定
	if client.UserID != "" {
		m.respondError(w, http.StatusConflict, "client already bound to user: "+client.UserID)
		return
	}

	// 4. 更新客户端信息
	client.UserID = req.UserID
	client.Type = models.ClientTypeRegistered
	// 如果提供了名称，则更新名称
	if req.Name != "" {
		client.Name = req.Name
	}

	if err := m.cloudControl.UpdateClient(client); err != nil {
		corelog.Errorf("ManagementModule: failed to bind client %d: %v", clientID, err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 5. 迁移相关的 PortMapping
	mappings, err := m.cloudControl.GetClientPortMappings(clientID)
	if err != nil {
		corelog.Warnf("ManagementModule: failed to get client mappings for migration: %v", err)
	} else {
		migratedCount := 0
		for _, mapping := range mappings {
			// 只迁移未绑定用户的映射
			if mapping.UserID == "" {
				mapping.UserID = req.UserID
				if err := m.cloudControl.UpdatePortMapping(mapping); err != nil {
					corelog.Warnf("ManagementModule: failed to migrate mapping %s: %v", mapping.ID, err)
				} else {
					migratedCount++
				}
			}
		}
		if migratedCount > 0 {
			corelog.Infof("ManagementModule: migrated %d mappings for client %d to user %s", migratedCount, clientID, req.UserID)
		}
	}

	corelog.Infof("ManagementModule: client %d bound to user %s", clientID, req.UserID)
	respondJSONTyped(w, http.StatusOK, client)
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
