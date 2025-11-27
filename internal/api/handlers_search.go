package api

import (
	"net/http"
)

// handleSearchUsers 搜索用户
func (s *ManagementAPIServer) handleSearchUsers(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("q")
	if keyword == "" {
		s.respondError(w, http.StatusBadRequest, "search keyword 'q' is required")
		return
	}
	
	users, err := s.cloudControl.SearchUsers(keyword)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	response := UserListResponse{
		Users: users,
		Total: len(users),
	}
	s.respondJSON(w, http.StatusOK, response)
}

// handleSearchClients 搜索客户端
func (s *ManagementAPIServer) handleSearchClients(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("q")
	if keyword == "" {
		s.respondError(w, http.StatusBadRequest, "search keyword 'q' is required")
		return
	}
	
	clients, err := s.cloudControl.SearchClients(keyword)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	response := ClientListResponse{
		Clients: clients,
		Total:   len(clients),
	}
	s.respondJSON(w, http.StatusOK, response)
}

// handleSearchMappings 搜索端口映射
func (s *ManagementAPIServer) handleSearchMappings(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("q")
	if keyword == "" {
		s.respondError(w, http.StatusBadRequest, "search keyword 'q' is required")
		return
	}
	
	mappings, err := s.cloudControl.SearchPortMappings(keyword)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	response := MappingListResponse{
		Mappings: mappings,
		Total:    len(mappings),
	}
	s.respondJSON(w, http.StatusOK, response)
}

