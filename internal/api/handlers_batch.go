package api

import (
	"fmt"
	"net/http"
	"tunnox-core/internal/cloud/models"
)

// BatchDisconnectRequest 批量下线请求
type BatchDisconnectRequest struct {
	ClientIDs []int64 `json:"client_ids"`
	Reason    string  `json:"reason,omitempty"`
}

// BatchDeleteMappingsRequest 批量删除映射请求
type BatchDeleteMappingsRequest struct {
	MappingIDs []string `json:"mapping_ids"`
}

// BatchUpdateMappingsRequest 批量更新映射请求
type BatchUpdateMappingsRequest struct {
	MappingIDs []string `json:"mapping_ids"`
	Status     string   `json:"status,omitempty"`
}

// BatchOperationResult 批量操作结果
type BatchOperationResult struct {
	SuccessCount int      `json:"success_count"`
	FailureCount int      `json:"failure_count"`
	SuccessIDs   []string `json:"success_ids"`
	FailureIDs   []string `json:"failure_ids"`
	Errors       []string `json:"errors,omitempty"`
}

// handleBatchDisconnectClients 批量下线客户端
func (s *ManagementAPIServer) handleBatchDisconnectClients(w http.ResponseWriter, r *http.Request) {
	var req BatchDisconnectRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	if len(req.ClientIDs) == 0 {
		s.respondError(w, http.StatusBadRequest, "client_ids is required")
		return
	}
	
	reason := req.Reason
	if reason == "" {
		reason = "Batch disconnection by administrator"
	}
	
	result := &BatchOperationResult{
		SuccessIDs: make([]string, 0),
		FailureIDs: make([]string, 0),
		Errors:     make([]string, 0),
	}
	
	for _, clientID := range req.ClientIDs {
		// 发送踢下线命令
		s.kickClient(clientID, reason, "ADMIN_BATCH_DISCONNECT")
		
		// 更新客户端状态
		if err := s.cloudControl.UpdateClientStatus(clientID, models.ClientStatusOffline, ""); err != nil {
			result.FailureCount++
			result.FailureIDs = append(result.FailureIDs, fmt.Sprintf("%d", clientID))
			result.Errors = append(result.Errors, err.Error())
		} else {
			result.SuccessCount++
			result.SuccessIDs = append(result.SuccessIDs, fmt.Sprintf("%d", clientID))
		}
	}
	
	s.respondJSON(w, http.StatusOK, result)
}

// handleBatchDeleteMappings 批量删除映射
func (s *ManagementAPIServer) handleBatchDeleteMappings(w http.ResponseWriter, r *http.Request) {
	var req BatchDeleteMappingsRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	if len(req.MappingIDs) == 0 {
		s.respondError(w, http.StatusBadRequest, "mapping_ids is required")
		return
	}
	
	result := &BatchOperationResult{
		SuccessIDs: make([]string, 0),
		FailureIDs: make([]string, 0),
		Errors:     make([]string, 0),
	}
	
	for _, mappingID := range req.MappingIDs {
		// 获取映射信息（用于推送删除通知）
		mapping, err := s.cloudControl.GetPortMapping(mappingID)
		if err != nil {
			result.FailureCount++
			result.FailureIDs = append(result.FailureIDs, mappingID)
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		
		// 删除映射
		if err := s.cloudControl.DeletePortMapping(mappingID); err != nil {
			result.FailureCount++
			result.FailureIDs = append(result.FailureIDs, mappingID)
			result.Errors = append(result.Errors, err.Error())
		} else {
			// 通知客户端移除映射
			s.removeMappingFromClients(mapping)
			
			result.SuccessCount++
			result.SuccessIDs = append(result.SuccessIDs, mappingID)
		}
	}
	
	s.respondJSON(w, http.StatusOK, result)
}

// handleBatchUpdateMappings 批量更新映射状态
func (s *ManagementAPIServer) handleBatchUpdateMappings(w http.ResponseWriter, r *http.Request) {
	var req BatchUpdateMappingsRequest
	if err := parseJSONBody(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	if len(req.MappingIDs) == 0 {
		s.respondError(w, http.StatusBadRequest, "mapping_ids is required")
		return
	}
	
	if req.Status == "" {
		s.respondError(w, http.StatusBadRequest, "status is required")
		return
	}
	
	result := &BatchOperationResult{
		SuccessIDs: make([]string, 0),
		FailureIDs: make([]string, 0),
		Errors:     make([]string, 0),
	}
	
	for _, mappingID := range req.MappingIDs {
		// 更新映射状态
		if err := s.cloudControl.UpdatePortMappingStatus(mappingID, models.MappingStatus(req.Status)); err != nil {
			result.FailureCount++
			result.FailureIDs = append(result.FailureIDs, mappingID)
			result.Errors = append(result.Errors, err.Error())
		} else {
			result.SuccessCount++
			result.SuccessIDs = append(result.SuccessIDs, mappingID)
			
			// 获取更新后的映射并推送配置
			if mapping, err := s.cloudControl.GetPortMapping(mappingID); err == nil {
				s.pushMappingToClients(mapping)
			}
		}
	}
	
	s.respondJSON(w, http.StatusOK, result)
}

