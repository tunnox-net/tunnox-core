package api

import (
	"net/http"
	"strconv"
	"tunnox-core/internal/cloud/models"
)

// handleListAllClients 列出所有客户端
func (s *ManagementAPIServer) handleListAllClients(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	clientType := r.URL.Query().Get("type") // registered / anonymous
	status := r.URL.Query().Get("status")   // online / offline / blocked

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
//
// 查询参数：
//   - client_id: 查询指定客户端的映射（可选）
//   - direction: outbound（出站，作为 ListenClient）| inbound（入站，作为 TargetClient）（可选）
//   - type: 映射类型过滤（可选）
//   - status: 映射状态过滤（可选）
func (s *ManagementAPIServer) handleListAllMappings(w http.ResponseWriter, r *http.Request) {
	// 获取查询参数
	clientIDStr := r.URL.Query().Get("client_id")
	direction := r.URL.Query().Get("direction") // outbound | inbound
	mappingType := r.URL.Query().Get("type")    // tcp / udp / socks5
	status := r.URL.Query().Get("status")       // active / inactive

	var mappings []*models.PortMapping
	var err error

	if clientIDStr != "" {
		// ✅ 查询指定客户端的映射
		clientID, err := strconv.ParseInt(clientIDStr, 10, 64)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, "invalid client_id")
			return
		}

		allMappings, err := s.cloudControl.GetClientPortMappings(clientID)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// 根据 direction 过滤
		if direction == "outbound" {
			// 只返回作为 ListenClient 的映射
			for _, m := range allMappings {
				if m.ListenClientID == clientID {
					mappings = append(mappings, m)
				}
			}
		} else if direction == "inbound" {
			// 只返回作为 TargetClient 的映射
			for _, m := range allMappings {
				if m.TargetClientID == clientID {
					mappings = append(mappings, m)
				}
			}
		} else {
			// 返回所有相关映射（已去重，因为 GetClientPortMappings 会去重）
			mappings = allMappings
		}
	} else {
		// 查询所有映射
		if mappingType != "" {
			mappings, err = s.cloudControl.ListPortMappings(models.MappingType(mappingType))
		} else {
			mappings, err = s.cloudControl.ListPortMappings("")
		}
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
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
