package api

import (
corelog "tunnox-core/internal/core/log"
	"encoding/json"
	"fmt"
	"net/http"

	"tunnox-core/internal/client"
)

// DebugAPIServer 客户端调试 API 服务器
type DebugAPIServer struct {
	client *client.TunnoxClient
	server *http.Server
	port   int
}

// NewDebugAPIServer 创建调试 API 服务器
func NewDebugAPIServer(client *client.TunnoxClient, port int) *DebugAPIServer {
	return &DebugAPIServer{
		client: client,
		port:   port,
	}
}

// Start 启动 API 服务器
func (s *DebugAPIServer) Start() error {
	mux := http.NewServeMux()

	// 状态相关
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/connect", s.handleConnect)
	mux.HandleFunc("/api/v1/disconnect", s.handleDisconnect)

	// 连接码相关
	mux.HandleFunc("/api/v1/codes/generate", s.handleGenerateCode)
	mux.HandleFunc("/api/v1/codes/list", s.handleListCodes)

	// 映射相关
	mux.HandleFunc("/api/v1/mappings/use-code", s.handleUseCode)
	mux.HandleFunc("/api/v1/mappings/list", s.handleListMappings)
	mux.HandleFunc("/api/v1/mappings/show", s.handleShowMapping)
	mux.HandleFunc("/api/v1/mappings/delete", s.handleDeleteMapping)

	// 配置相关
	mux.HandleFunc("/api/v1/config/list", s.handleConfigList)
	mux.HandleFunc("/api/v1/config/get", s.handleConfigGet)
	mux.HandleFunc("/api/v1/config/set", s.handleConfigSet)

	s.server = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler: mux,
	}

	corelog.Infof("Client Debug API: starting on http://127.0.0.1:%d", s.port)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			corelog.Errorf("Client Debug API: failed to start: %v", err)
		}
	}()

	return nil
}

// Stop 停止 API 服务器
func (s *DebugAPIServer) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// handleStatus 处理状态查询
func (s *DebugAPIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := s.client.GetStatus()
	response := map[string]interface{}{
		"connected":    status.Connected,
		"client_id":    status.ClientID,
		"device_id":    status.DeviceID,
		"server_addr":  status.ServerAddr,
		"protocol":     status.Protocol,
		"uptime":       status.Uptime.String(),
		"mapping_count": status.MappingCount,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleConnect 处理连接请求
func (s *DebugAPIServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := s.client.Connect()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to connect: %v", err))
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"status": "connected"})
}

// handleDisconnect 处理断开连接请求
func (s *DebugAPIServer) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.client.Disconnect()
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

// handleGenerateCode 处理生成连接码请求
func (s *DebugAPIServer) handleGenerateCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req client.GenerateConnectionCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	resp, err := s.client.GenerateConnectionCode(&req)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to generate code: %v", err))
		return
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleListCodes 处理列出连接码请求
func (s *DebugAPIServer) handleListCodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp, err := s.client.ListConnectionCodes()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list codes: %v", err))
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"codes": resp.Codes,
		"total": resp.Total,
	})
}

// handleUseCode 处理使用连接码请求
func (s *DebugAPIServer) handleUseCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	if req.Code == "" {
		s.writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	activateReq := &client.ActivateConnectionCodeRequest{
		Code: req.Code,
	}
	resp, err := s.client.ActivateConnectionCode(activateReq)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to use code: %v", err))
		return
	}

	s.writeJSON(w, http.StatusOK, resp)
}

// handleListMappings 处理列出映射请求
func (s *DebugAPIServer) handleListMappings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mappingType := r.URL.Query().Get("type")
	listReq := &client.ListMappingsRequest{}
	if mappingType != "" {
		listReq.Type = mappingType
	}
	resp, err := s.client.ListMappings(listReq)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list mappings: %v", err))
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"mappings": resp.Mappings,
		"total":    resp.Total,
	})
}

// handleShowMapping 处理显示映射详情请求
func (s *DebugAPIServer) handleShowMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mappingID := r.URL.Query().Get("id")
	if mappingID == "" {
		s.writeError(w, http.StatusBadRequest, "id parameter is required")
		return
	}

	mapping, err := s.client.GetMapping(mappingID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get mapping: %v", err))
		return
	}

	s.writeJSON(w, http.StatusOK, mapping)
}

// handleDeleteMapping 处理删除映射请求
func (s *DebugAPIServer) handleDeleteMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mappingID := r.URL.Query().Get("id")
	if mappingID == "" {
		s.writeError(w, http.StatusBadRequest, "id parameter is required")
		return
	}

	err := s.client.DeleteMapping(mappingID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete mapping: %v", err))
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleConfigList 处理列出配置请求
func (s *DebugAPIServer) handleConfigList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: 实现配置列表
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "config list not implemented yet",
	})
}

// handleConfigGet 处理获取配置请求
func (s *DebugAPIServer) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		s.writeError(w, http.StatusBadRequest, "key parameter is required")
		return
	}

	// TODO: 实现配置获取
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"key":   key,
		"value": "not implemented yet",
	})
}

// handleConfigSet 处理设置配置请求
func (s *DebugAPIServer) handleConfigSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	// TODO: 实现配置设置
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "not implemented yet"})
}

// writeJSON 写入 JSON 响应
func (s *DebugAPIServer) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		corelog.Errorf("Debug API: failed to encode JSON response: %v", err)
	}
}

// writeError 写入错误响应
func (s *DebugAPIServer) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{
		"error": message,
	})
}

