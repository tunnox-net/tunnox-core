package api

import (
	"net/http"
)

// handleListAllConnections 列出所有连接
func (s *ManagementAPIServer) handleListAllConnections(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	mappingID := r.URL.Query().Get("mapping_id")
	
	if mappingID != "" {
		// 列出指定映射的连接
		connections, err := s.cloudControl.GetConnections(mappingID)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"connections": connections,
			"total":       len(connections),
			"mapping_id":  mappingID,
		})
		return
	}
	
	// TODO: 实现列出所有连接的功能（需要在CloudControl中添加方法）
	s.respondError(w, http.StatusNotImplemented, "list all connections not yet implemented")
}

// handleListMappingConnections 列出映射的所有连接
func (s *ManagementAPIServer) handleListMappingConnections(w http.ResponseWriter, r *http.Request) {
	mappingID, err := getStringPathVar(r, "mapping_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	connections, err := s.cloudControl.GetConnections(mappingID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"connections": connections,
		"total":       len(connections),
		"mapping_id":  mappingID,
	})
}

// handleListClientConnections 列出客户端的所有连接
func (s *ManagementAPIServer) handleListClientConnections(w http.ResponseWriter, r *http.Request) {
	clientID, err := getInt64PathVar(r, "client_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	connections, err := s.cloudControl.GetClientConnections(clientID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"connections": connections,
		"total":       len(connections),
		"client_id":   clientID,
	})
}

// handleCloseConnection 强制关闭连接
func (s *ManagementAPIServer) handleCloseConnection(w http.ResponseWriter, r *http.Request) {
	connID, err := getStringPathVar(r, "conn_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	
	// 注销连接（会触发连接关闭）
	if err := s.cloudControl.UnregisterConnection(connID); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	s.respondJSON(w, http.StatusOK, map[string]string{
		"message": "Connection closed successfully",
		"conn_id": connID,
	})
}

