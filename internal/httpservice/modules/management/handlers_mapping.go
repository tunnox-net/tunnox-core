package management

import (
	"net/http"

	"tunnox-core/internal/cloud/models"
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
	Description string `json:"description,omitempty"`
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

	respondJSONTyped(w, http.StatusOK, mappings)
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
