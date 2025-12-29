package server

import (
	"fmt"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
)

// ServerTunnelHandler 服务器隧道处理器
type ServerTunnelHandler struct {
	cloudControl    managers.CloudControlAPI
	connCodeService *services.ConnectionCodeService
}

// NewServerTunnelHandler 创建隧道处理器
func NewServerTunnelHandler(cloudControl managers.CloudControlAPI, connCodeService *services.ConnectionCodeService) *ServerTunnelHandler {
	return &ServerTunnelHandler{
		cloudControl:    cloudControl,
		connCodeService: connCodeService,
	}
}

// HandleTunnelOpen 处理隧道打开请求
//
// 验证优先级：
//  1. MappingID - 验证隧道映射权限（新设计）
//  2. SecretKey - 传统密钥验证（向后兼容）
func (h *ServerTunnelHandler) HandleTunnelOpen(conn session.ControlConnectionInterface, req *packet.TunnelOpenRequest) error {

	// ✨ Phase 2: 优先级0 - 检查是否是恢复请求
	if req.ResumeToken != "" {
		return h.resumeTunnel(conn, req)
	}

	// 1. 验证ClientID
	if conn.GetClientID() == 0 {
		corelog.Warnf("ServerTunnelHandler: client not authenticated for connection %s", conn.GetConnID())
		return fmt.Errorf("client not authenticated")
	}

	var mapping *models.PortMapping

	// 2. 优先级1: MappingID验证（新设计，推荐）
	if req.MappingID != "" && req.SecretKey == "" {

		// 验证隧道映射权限
		if h.connCodeService == nil {
			corelog.Errorf("ServerTunnelHandler: connection code service not available")
			return fmt.Errorf("connection code service not available")
		}

		validatedMapping, err := h.connCodeService.ValidateMapping(req.MappingID, conn.GetClientID())
		if err != nil {
			corelog.Warnf("ServerTunnelHandler: mapping validation failed for %s (client %d): %v",
				req.MappingID, conn.GetClientID(), err)
			return fmt.Errorf("mapping validation failed: %w", err)
		}

		mapping = validatedMapping

		// 记录映射使用（统计）
		if err := h.connCodeService.RecordMappingUsage(req.MappingID); err != nil {
			corelog.Warnf("ServerTunnelHandler: failed to record mapping usage for %s: %v", req.MappingID, err)
			// 不返回错误，只记录警告
		}

	} else if req.SecretKey != "" {
		// 优先级2: SecretKey验证（向后兼容，用于旧版API调用）

		// 从旧的PortMapping获取（保持向后兼容）
		portMapping, err := h.cloudControl.GetPortMapping(req.MappingID)
		if err != nil {
			corelog.Errorf("ServerTunnelHandler: port mapping not found %s: %v", req.MappingID, err)
			return fmt.Errorf("mapping not found: %w", err)
		}

		// 验证SecretKey
		if err := h.validateWithSecretKey(req.SecretKey, portMapping); err != nil {
			corelog.Warnf("ServerTunnelHandler: secret key validation failed for mapping %s",
				req.MappingID)
			return fmt.Errorf("invalid secret key")
		}

		// ✅ 验证客户端是否有权限使用这个mapping
		// 只有 ListenClient 或 TargetClient 可以使用此映射
		if portMapping.ListenClientID != conn.GetClientID() && portMapping.TargetClientID != conn.GetClientID() {
			corelog.Warnf("ServerTunnelHandler: client %d not authorized for mapping %s (listenClientID=%d, target=%d)",
				conn.GetClientID(), req.MappingID, portMapping.ListenClientID, portMapping.TargetClientID)
			return fmt.Errorf("client not authorized for this mapping")
		}

	} else {
		// 无有效凭证
		corelog.Warnf("ServerTunnelHandler: no valid credentials provided for connection %s",
			conn.GetConnID())
		return fmt.Errorf("authentication required: either mapping_id or secret_key must be provided")
	}

	// 存储mapping信息到conn（如果需要后续使用）
	_ = mapping // 暂时未使用，但保留以备后续扩展

	return nil
}

// validateWithSecretKey 使用秘钥验证（传统方式，向后兼容）
func (h *ServerTunnelHandler) validateWithSecretKey(secretKey string, mapping *models.PortMapping) error {
	if mapping.SecretKey != secretKey {
		return fmt.Errorf("invalid secret key")
	}
	return nil
}

// resumeTunnel 恢复中断的隧道
//
// 当客户端发送带有ResumeToken的TunnelOpenRequest时调用。
//
// 流程：
// 1. 验证ResumeToken并加载隧道状态
// 2. 恢复隧道元数据（MappingID等）
// 3. 恢复缓冲区状态（如果启用序列号）
// 4. 返回成功，客户端可继续传输
func (h *ServerTunnelHandler) resumeTunnel(conn session.ControlConnectionInterface, req *packet.TunnelOpenRequest) error {
	corelog.Infof("ServerTunnelHandler: attempting to resume tunnel %s for client %d",
		req.TunnelID, conn.GetClientID())

	// 需要SessionManager支持
	sessionMgr, ok := h.cloudControl.(interface {
		ValidateTunnelResumeToken(string) (*session.TunnelState, error)
	})
	if !ok {
		corelog.Errorf("ServerTunnelHandler: session manager does not support tunnel resumption")
		return fmt.Errorf("tunnel resumption not supported")
	}

	// 1. 验证ResumeToken并加载隧道状态
	tunnelState, err := sessionMgr.ValidateTunnelResumeToken(req.ResumeToken)
	if err != nil {
		corelog.Warnf("ServerTunnelHandler: failed to validate resume token for tunnel %s: %v",
			req.TunnelID, err)
		return fmt.Errorf("invalid resume token: %w", err)
	}

	// 2. 验证TunnelID匹配
	if tunnelState.TunnelID != req.TunnelID {
		corelog.Warnf("ServerTunnelHandler: tunnel ID mismatch (token=%s, request=%s)",
			tunnelState.TunnelID, req.TunnelID)
		return fmt.Errorf("tunnel ID mismatch")
	}

	// 3. （可选）验证MappingID权限
	if h.connCodeService != nil && tunnelState.MappingID != "" {
		_, err := h.connCodeService.ValidateMapping(tunnelState.MappingID, conn.GetClientID())
		if err != nil {
			corelog.Warnf("ServerTunnelHandler: mapping validation failed during resume for %s: %v",
				tunnelState.MappingID, err)
			return fmt.Errorf("mapping validation failed: %w", err)
		}
	}

	// 4. 记录恢复成功（日志写入文件）
	corelog.Infof("ServerTunnelHandler: tunnel resumed successfully - TunnelID=%s, MappingID=%s, Client=%d",
		tunnelState.TunnelID, tunnelState.MappingID, conn.GetClientID())

	// 注意：缓冲区恢复功能已实现（见 session.RestoreToSendBuffer），
	// 如需启用，可在此处调用：session.RestoreToSendBuffer(tunnelConn.sendBuffer, tunnelState.BufferedPackets)

	return nil
}
