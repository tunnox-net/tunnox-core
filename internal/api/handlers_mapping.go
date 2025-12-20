package api

import (
corelog "tunnox-core/internal/core/log"
	"net/http"
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/models"
)

// CreateMappingRequest 创建端口映射请求
type CreateMappingRequest struct {
	UserID         string `json:"user_id"`
	ListenClientID int64  `json:"listen_client_id"`           // ✅ 统一命名
	SourceClientID int64  `json:"source_client_id,omitempty"` // ⚠️ 已废弃，向后兼容
	TargetClientID int64  `json:"target_client_id"`
	Protocol       string `json:"protocol"`
	SourcePort     int    `json:"source_port"` // 源端口
	TargetHost     string `json:"target_host"`
	TargetPort     int    `json:"target_port"`

	// 商业化控制
	BandwidthLimit int64 `json:"bandwidth_limit,omitempty"` // bytes/s
	MaxConnections int   `json:"max_connections,omitempty"` // 最大并发连接

	// 压缩和加密（密钥由服务器自动生成，不允许外部指定）
	EnableCompression bool   `json:"enable_compression,omitempty"`
	CompressionLevel  int    `json:"compression_level,omitempty"` // 0-9
	EnableEncryption  bool   `json:"enable_encryption,omitempty"`
	EncryptionMethod  string `json:"encryption_method,omitempty"` // aes-256-gcm, aes-128-gcm
}

// UpdateMappingRequest 更新端口映射请求
type UpdateMappingRequest struct {
	Status  string `json:"status,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// handleCreateMapping 创建端口映射
func (s *ManagementAPIServer) handleCreateMapping(w http.ResponseWriter, r *http.Request) {
	var req CreateMappingRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// ✅ 统一使用 ListenClientID（向后兼容：如果为 0 则使用 SourceClientID）
	listenClientID := req.ListenClientID
	if listenClientID == 0 {
		listenClientID = req.SourceClientID
	}

	// 验证必填字段（UserID允许为空，用于匿名客户端）
	if listenClientID == 0 || req.TargetClientID == 0 {
		s.respondError(w, http.StatusBadRequest, "listen_client_id (or source_client_id) and target_client_id are required")
		return
	}
	if req.Protocol == "" || req.TargetHost == "" || req.TargetPort == 0 {
		s.respondError(w, http.StatusBadRequest, "protocol, target_host, and target_port are required")
		return
	}

	// 构造端口映射对象
	mapping := &models.PortMapping{
		ListenClientID: listenClientID,     // ✅ 统一使用 ListenClientID
		SourceClientID: req.SourceClientID, // 保留用于向后兼容
		TargetClientID: req.TargetClientID,
		UserID:         req.UserID,
		Protocol:       models.Protocol(req.Protocol),
		TargetHost:     req.TargetHost,
		TargetPort:     req.TargetPort,
		SourcePort:     req.SourcePort,
		Status:         models.MappingStatusActive,
		Config: configs.MappingConfig{
			EnableCompression: req.EnableCompression,
			CompressionLevel:  req.CompressionLevel,
			EnableEncryption:  req.EnableEncryption,
			EncryptionMethod:  req.EncryptionMethod,
			// EncryptionKey 由 CloudControl 自动生成（运行时创建）
			BandwidthLimit: req.BandwidthLimit,
			MaxConnections: req.MaxConnections,
		},
	}

	// 创建端口映射（带事务保护）
	createdMapping, err := s.createMappingWithTransaction(mapping)
	if err != nil {
		corelog.Errorf("API: failed to create mapping: %v", err)
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusCreated, createdMapping)
}

// handleGetMapping 获取端口映射信息
func (s *ManagementAPIServer) handleGetMapping(w http.ResponseWriter, r *http.Request) {
	mappingID, err := getStringPathVar(r, "mapping_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	mapping, err := s.cloudControl.GetPortMapping(mappingID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, mapping)
}

// handleUpdateMapping 更新端口映射
func (s *ManagementAPIServer) handleUpdateMapping(w http.ResponseWriter, r *http.Request) {
	mappingID, err := getStringPathVar(r, "mapping_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdateMappingRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 获取现有映射
	mapping, err := s.cloudControl.GetPortMapping(mappingID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// 更新字段
	if req.Status != "" {
		mapping.Status = models.MappingStatus(req.Status)
	}
	// Note: Enabled field doesn't exist in current models.PortMapping

	// 保存更新
	if err := s.cloudControl.UpdatePortMapping(mapping); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 推送更新后的配置给客户端
	s.pushMappingToClients(mapping)

	s.respondJSON(w, http.StatusOK, mapping)
}

// handleDeleteMapping 删除端口映射
func (s *ManagementAPIServer) handleDeleteMapping(w http.ResponseWriter, r *http.Request) {
	mappingID, err := getStringPathVar(r, "mapping_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 先获取映射信息（用于推送删除通知）
	mapping, err := s.cloudControl.GetPortMapping(mappingID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// 删除映射
	if err := s.cloudControl.DeletePortMapping(mappingID); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 通知客户端移除映射
	s.removeMappingFromClients(mapping)

	w.WriteHeader(http.StatusNoContent)
}
