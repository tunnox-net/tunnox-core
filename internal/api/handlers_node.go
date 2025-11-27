package api

import (
	"net/http"
)

// handleListNodes 获取在线节点列表
func (s *ManagementAPIServer) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.cloudControl.GetAllNodeServiceInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := NodeListResponse{
		Nodes: nodes,
		Total: len(nodes),
	}
	s.respondJSON(w, http.StatusOK, response)
}

// handleGetNode 获取节点详情
func (s *ManagementAPIServer) handleGetNode(w http.ResponseWriter, r *http.Request) {
	nodeID, err := getStringPathVar(r, "node_id")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	node, err := s.cloudControl.GetNodeServiceInfo(nodeID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, node)
}

