package management

import (
	"net/http"

	"tunnox-core/internal/cloud/models"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
)

// CreateMappingRequest 创建映射请求
type CreateMappingRequest struct {
	ListenClientID int64  `json:"listen_client_id"`
	TargetClientID int64  `json:"target_client_id"`
	Protocol       string `json:"protocol"`
	TargetHost     string `json:"target_host"`
	TargetPort     int    `json:"target_port"`
	SourcePort     int    `json:"source_port"`

	// HTTP 域名映射字段
	HTTPSubdomain  string `json:"http_subdomain,omitempty"`
	HTTPBaseDomain string `json:"http_base_domain,omitempty"`

	// 可选字段
	Name        string `json:"name,omitempty"`        // 隧道名称
	Description string `json:"description,omitempty"` // 隧道描述
	UserID      string `json:"user_id,omitempty"`     // 用户ID（用于配额检查）
}

// handleListAllMappings 列出所有映射
func (m *ManagementModule) handleListAllMappings(w http.ResponseWriter, r *http.Request) {
	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	mappings, err := m.cloudControl.ListPortMappings("")
	if err != nil {
		corelog.Errorf("ManagementModule: failed to list all mappings: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 确保返回空数组而非 nil
	if mappings == nil {
		mappings = []*models.PortMapping{}
	}

	// 包装成对象返回，符合 platform 期望的格式
	respondJSONTyped(w, http.StatusOK, map[string]interface{}{
		"mappings": mappings,
	})
}

// handleCreateMapping 创建映射
func (m *ManagementModule) handleCreateMapping(w http.ResponseWriter, r *http.Request) {
	var req CreateMappingRequest
	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	protocol := models.Protocol(req.Protocol)

	// 配额检查（云服务模式）
	if m.quotaChecker != nil && req.UserID != "" {
		if err := m.quotaChecker.CheckMappingQuota(req.UserID, protocol); err != nil {
			if coreerrors.IsCode(err, coreerrors.CodeQuotaExceeded) {
				m.respondError(w, http.StatusForbidden, err.Error())
			} else {
				corelog.Warnf("ManagementModule: quota check failed: %v", err)
				m.respondError(w, http.StatusInternalServerError, "quota check failed")
			}
			return
		}
	}

	// 构建 PortMapping
	mapping := &models.PortMapping{
		ListenClientID: req.ListenClientID,
		TargetClientID: req.TargetClientID,
		Protocol:       protocol,
		SourcePort:     req.SourcePort,
		TargetHost:     req.TargetHost,
		TargetPort:     req.TargetPort,
		HTTPSubdomain:  req.HTTPSubdomain,
		HTTPBaseDomain: req.HTTPBaseDomain,
		Name:           req.Name,
		UserID:         req.UserID,
		Description:    req.Description,
		Status:         models.MappingStatusActive,
	}

	// 验证 HTTP 映射
	if protocol == models.ProtocolHTTP {
		if req.HTTPSubdomain == "" || req.HTTPBaseDomain == "" {
			m.respondError(w, http.StatusBadRequest, "http_subdomain and http_base_domain are required for HTTP protocol")
			return
		}

		// 检查域名是否可用
		if m.deps != nil && m.deps.DomainRegistry != nil {
			if !m.deps.DomainRegistry.IsSubdomainAvailable(req.HTTPSubdomain, req.HTTPBaseDomain) {
				m.respondError(w, http.StatusConflict, "subdomain already in use")
				return
			}
		}
	}

	// 创建映射
	created, err := m.cloudControl.CreatePortMapping(mapping)
	if err != nil {
		corelog.Errorf("ManagementModule: failed to create mapping: %v", err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 注册到域名注册表
	if protocol == models.ProtocolHTTP && m.deps != nil && m.deps.DomainRegistry != nil {
		// 检查基础域名是否允许
		if !m.deps.DomainRegistry.IsBaseDomainAllowed(req.HTTPBaseDomain) {
			// 回滚：删除已创建的映射（忽略删除错误，主流程已失败）
			_ = m.cloudControl.DeletePortMapping(created.ID)
			m.respondError(w, http.StatusBadRequest, "base domain not allowed: "+req.HTTPBaseDomain)
			return
		}

		if err := m.deps.DomainRegistry.Register(created); err != nil {
			corelog.Errorf("ManagementModule: failed to register domain: %v", err)
			// 回滚：删除已创建的映射（忽略删除错误，主流程已失败）
			_ = m.cloudControl.DeletePortMapping(created.ID)
			m.respondError(w, http.StatusInternalServerError, "failed to register domain: "+err.Error())
			return
		}
		corelog.Infof("ManagementModule: registered domain %s.%s", req.HTTPSubdomain, req.HTTPBaseDomain)
	}

	// 通知相关客户端更新配置
	corelog.Infof("ManagementModule: checking deps for notification, deps=%v, SessionMgr=%v",
		m.deps != nil, m.deps != nil && m.deps.SessionMgr != nil)
	if m.deps != nil && m.deps.SessionMgr != nil {
		// 通知 ListenClient（需要启动监听）
		if created.ListenClientID > 0 {
			corelog.Infof("ManagementModule: notifying listen client %d of new mapping", created.ListenClientID)
			m.deps.SessionMgr.NotifyClientUpdate(created.ListenClientID)
		}
		// 通知 TargetClient（如果不同于 ListenClient）
		if created.TargetClientID > 0 && created.TargetClientID != created.ListenClientID {
			corelog.Infof("ManagementModule: notifying target client %d of new mapping", created.TargetClientID)
			m.deps.SessionMgr.NotifyClientUpdate(created.TargetClientID)
		}
	} else {
		corelog.Warnf("ManagementModule: cannot notify clients, deps or SessionMgr is nil")
	}

	respondJSONTyped(w, http.StatusCreated, created)
}

// handleGetMapping 获取映射
func (m *ManagementModule) handleGetMapping(w http.ResponseWriter, r *http.Request) {
	mappingID, err := getStringPathVar(r, "mapping_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	mapping, err := m.cloudControl.GetPortMapping(mappingID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSONTyped(w, http.StatusOK, mapping)
}

// UpdateMappingRequest 更新映射请求
type UpdateMappingRequest struct {
	Status      models.MappingStatus `json:"status,omitempty"`
	Description string               `json:"description,omitempty"`
	TargetHost  string               `json:"target_host,omitempty"`
	TargetPort  int                  `json:"target_port,omitempty"`
}

// handleUpdateMapping 更新映射
func (m *ManagementModule) handleUpdateMapping(w http.ResponseWriter, r *http.Request) {
	mappingID, err := getStringPathVar(r, "mapping_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdateMappingRequest
	if err := parseJSONBody(r, &req); err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 获取现有映射
	existing, err := m.cloudControl.GetPortMapping(mappingID)
	if err != nil {
		m.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// 更新字段
	if req.Status != "" {
		existing.Status = req.Status
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.TargetHost != "" {
		existing.TargetHost = req.TargetHost
	}
	if req.TargetPort > 0 {
		existing.TargetPort = req.TargetPort
	}

	// 保存更新
	if err := m.cloudControl.UpdatePortMapping(existing); err != nil {
		corelog.Errorf("ManagementModule: failed to update mapping %s: %v", mappingID, err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 更新域名注册表
	if existing.Protocol == models.ProtocolHTTP && m.deps != nil && m.deps.DomainRegistry != nil {
		if err := m.deps.DomainRegistry.Register(existing); err != nil {
			corelog.Warnf("ManagementModule: failed to update domain registry: %v", err)
		}
	}

	// 通知相关客户端更新配置
	if m.deps != nil && m.deps.SessionMgr != nil {
		if existing.ListenClientID > 0 {
			m.deps.SessionMgr.NotifyClientUpdate(existing.ListenClientID)
		}
		if existing.TargetClientID > 0 && existing.TargetClientID != existing.ListenClientID {
			m.deps.SessionMgr.NotifyClientUpdate(existing.TargetClientID)
		}
	}

	respondJSONTyped(w, http.StatusOK, existing)
}

// handleDeleteMapping 删除映射
func (m *ManagementModule) handleDeleteMapping(w http.ResponseWriter, r *http.Request) {
	mappingID, err := getStringPathVar(r, "mapping_id")
	if err != nil {
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 获取映射信息（用于从域名注册表移除和通知客户端）
	// 错误被忽略是因为映射可能已不存在，后续通过 nil 检查处理
	mapping, err := m.cloudControl.GetPortMapping(mappingID)
	if err != nil {
		corelog.Debugf("ManagementModule: mapping %s not found (may already be deleted): %v", mappingID, err)
	}

	// 删除映射
	if err := m.cloudControl.DeletePortMapping(mappingID); err != nil {
		corelog.Errorf("ManagementModule: failed to delete mapping %s: %v", mappingID, err)
		m.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 从域名注册表移除
	if mapping != nil && mapping.Protocol == models.ProtocolHTTP && m.deps != nil && m.deps.DomainRegistry != nil {
		m.deps.DomainRegistry.UnregisterByMappingID(mappingID)
	}

	// 通知相关客户端更新配置
	if mapping != nil && m.deps != nil && m.deps.SessionMgr != nil {
		if mapping.ListenClientID > 0 {
			m.deps.SessionMgr.NotifyClientUpdate(mapping.ListenClientID)
		}
		if mapping.TargetClientID > 0 && mapping.TargetClientID != mapping.ListenClientID {
			m.deps.SessionMgr.NotifyClientUpdate(mapping.TargetClientID)
		}
	}

	respondJSONTyped(w, http.StatusOK, httpservice.MessageResponse{Message: "mapping deleted"})
}

// handleCleanupOrphanedMapping 清理孤立的映射索引
// 当主数据不存在但索引中仍有残留时，由 Platform 调用此 API 进行清理
func (m *ManagementModule) handleCleanupOrphanedMapping(w http.ResponseWriter, r *http.Request) {
	corelog.Infof("ManagementModule: handleCleanupOrphanedMapping called")
	var req struct {
		MappingID string                 `json:"mapping_id"`
		UserID    string                 `json:"user_id"`
		Mapping   map[string]interface{} `json:"mapping"`
	}

	if err := parseJSONBody(r, &req); err != nil {
		corelog.Errorf("ManagementModule: handleCleanupOrphanedMapping parse error: %v", err)
		m.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	corelog.Infof("ManagementModule: cleanup orphaned mapping request - mappingID=%s, userID=%s, mapping=%+v", req.MappingID, req.UserID, req.Mapping)

	if req.MappingID == "" || req.UserID == "" {
		m.respondError(w, http.StatusBadRequest, "mapping_id and user_id are required")
		return
	}

	if m.cloudControl == nil {
		m.respondError(w, http.StatusInternalServerError, "cloud control not configured")
		return
	}

	// 从 mapping 数据中提取需要的信息来清理索引
	if err := m.cloudControl.CleanupOrphanedMappingIndexes(req.MappingID, req.UserID, req.Mapping); err != nil {
		corelog.Warnf("ManagementModule: failed to cleanup orphaned mapping %s: %v", req.MappingID, err)
		// 即使清理失败也返回成功，因为主数据已不存在
	} else {
		corelog.Infof("ManagementModule: successfully cleaned up orphaned mapping %s", req.MappingID)
	}

	respondJSONTyped(w, http.StatusOK, httpservice.MessageResponse{Message: "orphaned mapping cleaned up"})
}

// handleCheckSubdomain 检查子域名可用性
func (m *ManagementModule) handleCheckSubdomain(w http.ResponseWriter, r *http.Request) {
	subdomain := r.URL.Query().Get("subdomain")
	baseDomain := r.URL.Query().Get("base_domain")

	if subdomain == "" || baseDomain == "" {
		m.respondError(w, http.StatusBadRequest, "subdomain and base_domain are required")
		return
	}

	available := true
	if m.deps != nil && m.deps.DomainRegistry != nil {
		available = m.deps.DomainRegistry.IsSubdomainAvailable(subdomain, baseDomain)
	}

	respondJSONTyped(w, http.StatusOK, httpservice.SubdomainCheckResponse{
		Available:  available,
		FullDomain: subdomain + "." + baseDomain,
	})
}
