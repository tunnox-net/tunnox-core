package api

import (
	"net/http"
	"tunnox-core/internal/cloud/models"
)

// CreateMappingRequest 创建端口映射请求
type CreateMappingRequest struct {
	UserID           string `json:"user_id"`
	SourceClientID   int64  `json:"source_client_id"`
	TargetClientID   int64  `json:"target_client_id"`
	Protocol         string `json:"protocol"`
	TargetHost       string `json:"target_host"`
	TargetPort       int    `json:"target_port"`
	LocalPort        int    `json:"local_port,omitempty"`
	EnableCompression bool  `json:"enable_compression,omitempty"`
	EnableEncryption  bool  `json:"enable_encryption,omitempty"`
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

	// 验证必填字段
	if req.UserID == "" || req.SourceClientID == 0 || req.TargetClientID == 0 {
		s.respondError(w, http.StatusBadRequest, "user_id, source_client_id, and target_client_id are required")
		return
	}
	if req.Protocol == "" || req.TargetHost == "" || req.TargetPort == 0 {
		s.respondError(w, http.StatusBadRequest, "protocol, target_host, and target_port are required")
		return
	}

	// 构造端口映射对象
	mapping := &models.PortMapping{
		SourceClientID: req.SourceClientID,
		TargetClientID: req.TargetClientID,
		UserID:         req.UserID,
		Protocol:       models.Protocol(req.Protocol),
		TargetHost:     req.TargetHost,
		TargetPort:     req.TargetPort,
		SourcePort:     req.LocalPort,
		Status:         models.MappingStatusActive,
	}

	// 创建端口映射
	createdMapping, err := s.cloudControl.CreatePortMapping(mapping)
	if err != nil {
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

	s.respondJSON(w, http.StatusOK, mapping)
}

// handleDeleteMapping 删除端口映射
func (s *ManagementAPIServer) handleDeleteMapping(w http.ResponseWriter, r *http.Request) {
	mappingID, err := getStringPathVar(r, "mapping_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.cloudControl.DeletePortMapping(mappingID); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

