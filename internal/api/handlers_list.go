package api

import (
	"net/http"
	"tunnox-core/internal/cloud/models"
)

// handleListAllClients 列出所有客户端
func (s *ManagementAPIServer) handleListAllClients(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	clientType := r.URL.Query().Get("type")   // registered / anonymous
	status := r.URL.Query().Get("status")      // online / offline / blocked
	
	// 查询所有客户端
	var clients []*models.Client
	var err error
	
	if clientType != "" {
		clients, err = s.cloudControl.ListClients("", models.ClientType(clientType))
	} else {
		clients, err = s.cloudControl.ListClients("", "")
	}
	
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	// 根据状态过滤
	if status != "" {
		filteredClients := make([]*models.Client, 0)
		for _, client := range clients {
			if string(client.Status) == status {
				filteredClients = append(filteredClients, client)
			}
		}
		clients = filteredClients
	}
	
	response := ClientListResponse{
		Clients: clients,
		Total:   len(clients),
	}
	s.respondJSON(w, http.StatusOK, response)
}

// handleListAllMappings 列出所有端口映射
func (s *ManagementAPIServer) handleListAllMappings(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	mappingType := r.URL.Query().Get("type") // tcp / udp / socks5
	status := r.URL.Query().Get("status")     // active / inactive
	
	// 查询所有映射
	var mappings []*models.PortMapping
	var err error
	
	if mappingType != "" {
		mappings, err = s.cloudControl.ListPortMappings(models.MappingType(mappingType))
	} else {
		mappings, err = s.cloudControl.ListPortMappings("")
	}
	
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	// 根据状态过滤
	if status != "" {
		filteredMappings := make([]*models.PortMapping, 0)
		for _, mapping := range mappings {
			if string(mapping.Status) == status {
				filteredMappings = append(filteredMappings, mapping)
			}
		}
		mappings = filteredMappings
	}
	
	response := MappingListResponse{
		Mappings: mappings,
		Total:    len(mappings),
	}
	s.respondJSON(w, http.StatusOK, response)
}

