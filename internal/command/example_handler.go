package command

import (
	"fmt"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// TcpMapRequest TCP映射请求结构
type TcpMapRequest struct {
	SourcePort int    `json:"source_port"`
	TargetPort int    `json:"target_port"`
	TargetHost string `json:"target_host"`
	Protocol   string `json:"protocol"`
}

// TcpMapResponse TCP映射响应结构
type TcpMapResponse struct {
	MappingID  string `json:"mapping_id"`
	Status     string `json:"status"`
	LocalPort  int    `json:"local_port"`
	RemotePort int    `json:"remote_port"`
}

// TcpMapHandler TCP映射命令处理器
type TcpMapHandler struct {
	*BaseCommandHandler[TcpMapRequest, TcpMapResponse]
}

// NewTcpMapHandler 创建TCP映射处理器
func NewTcpMapHandler() *TcpMapHandler {
	base := NewBaseCommandHandler[TcpMapRequest, TcpMapResponse](
		packet.TcpMap,
		Oneway,  // 单向调用
		Simplex, // 单工模式
	)

	return &TcpMapHandler{
		BaseCommandHandler: base,
	}
}

// ValidateRequest 重写验证方法
func (h *TcpMapHandler) ValidateRequest(request *TcpMapRequest) error {
	if request.SourcePort <= 0 || request.SourcePort > 65535 {
		return fmt.Errorf("invalid source port: %d", request.SourcePort)
	}
	if request.TargetPort <= 0 || request.TargetPort > 65535 {
		return fmt.Errorf("invalid target port: %d", request.TargetPort)
	}
	if request.TargetHost == "" {
		return fmt.Errorf("target host is required")
	}
	return nil
}

// PreProcess 重写预处理方法
func (h *TcpMapHandler) PreProcess(ctx *CommandContext, request *TcpMapRequest) error {
	// 记录请求日志
	h.LogRequest(ctx, request)

	// 验证上下文
	if err := h.ValidateContext(ctx); err != nil {
		return err
	}

	utils.Infof("Preparing TCP mapping: %s:%d -> %s:%d",
		request.TargetHost, request.TargetPort,
		request.TargetHost, request.TargetPort)

	return nil
}

// ProcessRequest 实现核心处理逻辑
func (h *TcpMapHandler) ProcessRequest(ctx *CommandContext, request *TcpMapRequest) (*TcpMapResponse, error) {
	// 这里实现实际的TCP映射逻辑
	// 例如：创建端口映射、启动代理服务等

	mappingID := fmt.Sprintf("tcp_%s_%d_%d", ctx.ConnectionID, request.SourcePort, request.TargetPort)

	response := &TcpMapResponse{
		MappingID:  mappingID,
		Status:     "active",
		LocalPort:  request.SourcePort,
		RemotePort: request.TargetPort,
	}

	utils.Infof("TCP mapping created: %s", mappingID)

	return response, nil
}

// PostProcess 重写后处理方法
func (h *TcpMapHandler) PostProcess(ctx *CommandContext, response *TcpMapResponse) error {
	// 记录响应日志
	h.LogResponse(ctx, response, nil)

	utils.Infof("TCP mapping completed: %s", response.MappingID)

	return nil
}

// HttpMapRequest HTTP映射请求结构
type HttpMapRequest struct {
	Domain     string `json:"domain"`
	LocalPort  int    `json:"local_port"`
	SSLEnabled bool   `json:"ssl_enabled"`
}

// HttpMapResponse HTTP映射响应结构
type HttpMapResponse struct {
	MappingID  string `json:"mapping_id"`
	Status     string `json:"status"`
	PublicURL  string `json:"public_url"`
	LocalPort  int    `json:"local_port"`
	SSLEnabled bool   `json:"ssl_enabled"`
}

// HttpMapHandler HTTP映射命令处理器
type HttpMapHandler struct {
	*BaseCommandHandler[HttpMapRequest, HttpMapResponse]
}

// NewHttpMapHandler 创建HTTP映射处理器
func NewHttpMapHandler() *HttpMapHandler {
	base := NewBaseCommandHandler[HttpMapRequest, HttpMapResponse](
		packet.HttpMap,
		Duplex,     // 双工调用
		DuplexMode, // 双工模式
	)

	return &HttpMapHandler{
		BaseCommandHandler: base,
	}
}

// ValidateRequest 重写验证方法
func (h *HttpMapHandler) ValidateRequest(request *HttpMapRequest) error {
	if request.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	if request.LocalPort <= 0 || request.LocalPort > 65535 {
		return fmt.Errorf("invalid local port: %d", request.LocalPort)
	}
	return nil
}

// PreProcess 重写预处理方法
func (h *HttpMapHandler) PreProcess(ctx *CommandContext, request *HttpMapRequest) error {
	h.LogRequest(ctx, request)

	if err := h.ValidateContext(ctx); err != nil {
		return err
	}

	utils.Infof("Preparing HTTP mapping: %s -> localhost:%d (SSL: %v)",
		request.Domain, request.LocalPort, request.SSLEnabled)

	return nil
}

// ProcessRequest 实现核心处理逻辑
func (h *HttpMapHandler) ProcessRequest(ctx *CommandContext, request *HttpMapRequest) (*HttpMapResponse, error) {
	// 这里实现实际的HTTP映射逻辑
	// 例如：配置反向代理、SSL证书管理等

	mappingID := fmt.Sprintf("http_%s_%d", ctx.ConnectionID, request.LocalPort)

	protocol := "http"
	if request.SSLEnabled {
		protocol = "https"
	}

	publicURL := fmt.Sprintf("%s://%s", protocol, request.Domain)

	response := &HttpMapResponse{
		MappingID:  mappingID,
		Status:     "active",
		PublicURL:  publicURL,
		LocalPort:  request.LocalPort,
		SSLEnabled: request.SSLEnabled,
	}

	utils.Infof("HTTP mapping created: %s -> %s", publicURL, mappingID)

	return response, nil
}

// PostProcess 重写后处理方法
func (h *HttpMapHandler) PostProcess(ctx *CommandContext, response *HttpMapResponse) error {
	h.LogResponse(ctx, response, nil)

	utils.Infof("HTTP mapping completed: %s", response.MappingID)

	return nil
}
