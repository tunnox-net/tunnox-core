package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"tunnox-core/internal/bridge"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"

	"github.com/google/uuid"
)

// TunnelManager 隧道管理器（负责隧道生命周期和数据转发）
type TunnelManager struct {
	*dispose.ManagerBase

	tunnels   map[string]*Tunnel // tunnel_id -> Tunnel
	tunnelsMu sync.RWMutex

	sessionMgr    *session.SessionManager
	bridgeMgr     *bridge.BridgeManager
	messageBroker broker.MessageBroker
	cloudControl  *managers.CloudControl
	currentNodeID string
}

// NewTunnelManager 创建隧道管理器
func NewTunnelManager(
	ctx context.Context,
	sessionMgr *session.SessionManager,
	bridgeMgr *bridge.BridgeManager,
	messageBroker broker.MessageBroker,
	cloudControl *managers.CloudControl,
	nodeID string,
) *TunnelManager {
	tm := &TunnelManager{
		ManagerBase:   dispose.NewManager("TunnelManager", ctx),
		tunnels:       make(map[string]*Tunnel),
		sessionMgr:    sessionMgr,
		bridgeMgr:     bridgeMgr,
		messageBroker: messageBroker,
		cloudControl:  cloudControl,
		currentNodeID: nodeID,
	}

	go tm.cleanupLoop()

	return tm
}

// HandleTunnelOpen 处理隧道打开请求（映射连接认证）
// 区分两种情况：
// 1. 源客户端（ClientA）发起 - 创建新 Tunnel
// 2. 目标客户端（ClientB）响应 - 关联到现有 Tunnel
func (tm *TunnelManager) HandleTunnelOpen(conn *session.ClientConnection, req *packet.TunnelOpenRequest) error {
	utils.Infof("TunnelManager: handling tunnel open request, client_id=%d, mapping_id=%s, tunnel_id=%s",
		conn.ClientID, req.MappingID, req.TunnelID)

	// 1. 查询映射配置
	mapping, err := tm.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		utils.Errorf("TunnelManager: mapping not found, mapping_id=%s, error=%v", req.MappingID, err)
		return fmt.Errorf("mapping not found: %w", err)
	}

	// 2. ✅ 验证映射的固定秘钥（映射连接认证）
	if mapping.SecretKey == "" {
		utils.Warnf("TunnelManager: mapping has no secret key, mapping_id=%s", req.MappingID)
		return errors.New("mapping secret key not configured")
	}

	if req.SecretKey == "" {
		utils.Warnf("TunnelManager: no secret key provided in request, mapping_id=%s", req.MappingID)
		return errors.New("secret key required")
	}

	if mapping.SecretKey != req.SecretKey {
		utils.Warnf("TunnelManager: invalid secret key for mapping %s", req.MappingID)
		return errors.New("invalid secret key")
	}

	utils.Infof("TunnelManager: secret key validated successfully for mapping %s", req.MappingID)

	// 3. 映射状态检查
	if mapping.Status != models.MappingStatusActive {
		utils.Warnf("TunnelManager: mapping inactive, mapping_id=%s, status=%s", req.MappingID, mapping.Status)
		return errors.New("mapping inactive")
	}

	// 4. ✅ 判断是源端还是目标端
	isSourceClient := (conn.ClientID == mapping.SourceClientID)
	isTargetClient := (conn.ClientID == mapping.TargetClientID)

	if !isSourceClient && !isTargetClient {
		utils.Warnf("TunnelManager: client not authorized for this mapping, client_id=%d, mapping_id=%s",
			conn.ClientID, req.MappingID)
		return errors.New("not authorized for this mapping")
	}

	if isSourceClient {
		// ✅ 源端：创建新 Tunnel
		return tm.handleSourceTunnelOpen(conn, req, mapping)
	} else {
		// ✅ 目标端：关联到现有 Tunnel
		return tm.handleTargetTunnelOpen(conn, req, mapping)
	}
}

// handleSourceTunnelOpen 处理源客户端的 TunnelOpen（创建新 Tunnel）
func (tm *TunnelManager) handleSourceTunnelOpen(conn *session.ClientConnection, req *packet.TunnelOpenRequest, mapping *models.PortMapping) error {
	utils.Infof("TunnelManager: source client tunnel open, client_id=%d, tunnel_id=%s", conn.ClientID, req.TunnelID)

	// 1. 检查 Tunnel 是否已存在
	tm.tunnelsMu.RLock()
	existingTunnel := tm.tunnels[req.TunnelID]
	tm.tunnelsMu.RUnlock()

	if existingTunnel != nil {
		utils.Warnf("TunnelManager: tunnel already exists, tunnel_id=%s", req.TunnelID)
		return errors.New("tunnel already exists")
	}

	// 2. 创建新隧道
	tunnel := &Tunnel{
		TunnelID:       req.TunnelID,
		MappingID:      mapping.ID,
		SourceClientID: mapping.SourceClientID,
		TargetClientID: mapping.TargetClientID,
		SourceConn:     conn,
		CreatedAt:      time.Now(),
		LastActiveAt:   time.Now(),
	}

	// 5. 查询目标客户端在哪个节点
	targetClient, err := tm.cloudControl.GetClient(mapping.TargetClientID)
	if err != nil {
		utils.Errorf("TunnelManager: target client not found, client_id=%d, error=%v", mapping.TargetClientID, err)
		return fmt.Errorf("target client not found: %w", err)
	}

	// 3. 判断是否跨节点
	if targetClient.NodeID == tm.currentNodeID {
		// ✅ 本地转发：通知目标客户端建立连接
		utils.Infof("TunnelManager: local forwarding, tunnel_id=%s", req.TunnelID)
		tunnel.IsLocal = true

		// 检查目标客户端的指令连接是否在线
		targetControlConn := tm.sessionMgr.GetControlConnectionByClientID(mapping.TargetClientID)
		if targetControlConn == nil {
			utils.Errorf("TunnelManager: target client not connected, client_id=%d", mapping.TargetClientID)
			return fmt.Errorf("target client %d not connected", mapping.TargetClientID)
		}

		// ✅ 发送 TunnelOpenRequest 命令给目标客户端（通过指令连接）
		if err := tm.sendTunnelOpenRequestToTarget(targetControlConn, req.TunnelID, mapping); err != nil {
			utils.Errorf("TunnelManager: failed to send tunnel open request to target, error=%v", err)
			return fmt.Errorf("failed to notify target client: %w", err)
		}

	} else {
		// ✅ 跨节点转发：发布桥接请求
		utils.Infof("TunnelManager: cross-node forwarding, tunnel_id=%s, target_node=%s", req.TunnelID, targetClient.NodeID)
		tunnel.IsLocal = false

		// 发布桥接请求到 MessageBroker
		bridgeReq := &broker.BridgeRequestMessage{
			RequestID:      uuid.New().String(),
			SourceNodeID:   tm.currentNodeID,
			TargetNodeID:   targetClient.NodeID,
			SourceClientID: mapping.SourceClientID,
			TargetClientID: mapping.TargetClientID,
			TargetHost:     mapping.TargetHost,
			TargetPort:     mapping.TargetPort,
			TunnelID:       req.TunnelID,
			MappingID:      req.MappingID,
		}

		bridgeReqData, _ := json.Marshal(bridgeReq)
		if err := tm.messageBroker.Publish(tm.Ctx(), broker.TopicBridgeRequest, bridgeReqData); err != nil {
			utils.Errorf("TunnelManager: failed to publish bridge request, error=%v", err)
			return fmt.Errorf("failed to publish bridge request: %w", err)
		}

		utils.Infof("TunnelManager: bridge request published, request_id=%s", bridgeReq.RequestID)
	}

	// 4. ✅ 注册隧道（TargetConn 暂时为 nil，等目标端响应后填充）
	tm.tunnelsMu.Lock()
	tm.tunnels[req.TunnelID] = tunnel
	tm.tunnelsMu.Unlock()

	utils.Infof("TunnelManager: tunnel created (source side), tunnel_id=%s, is_local=%v", req.TunnelID, tunnel.IsLocal)

	// 5. 发送 TunnelOpenAck 给源客户端
	return tm.sendTunnelOpenAck(conn, req.TunnelID, true, "")
}

// ✅ HandleTunnelData 和 HandleTunnelClose 已删除
// 原因：前置包（TunnelOpen）处理完成后，直接切换到裸连接模式（io.Copy）
// 不再需要 TunnelData 和 TunnelClose 包

// AttachForwardSession 关联跨节点 ForwardSession（由 BridgeManager 调用）
func (tm *TunnelManager) AttachForwardSession(tunnelID string, session *bridge.ForwardSession) error {
	tm.tunnelsMu.Lock()
	defer tm.tunnelsMu.Unlock()

	tunnel := tm.tunnels[tunnelID]
	if tunnel == nil {
		return fmt.Errorf("tunnel %s not found", tunnelID)
	}

	tunnel.TargetConn = session
	utils.Infof("TunnelManager: forward session attached to tunnel %s", tunnelID)

	return nil
}

// GetTunnel 获取隧道
func (tm *TunnelManager) GetTunnel(tunnelID string) *Tunnel {
	tm.tunnelsMu.RLock()
	defer tm.tunnelsMu.RUnlock()
	return tm.tunnels[tunnelID]
}

// handleTargetTunnelOpen 处理目标客户端的 TunnelOpen（关联到现有 Tunnel）
func (tm *TunnelManager) handleTargetTunnelOpen(conn *session.ClientConnection, req *packet.TunnelOpenRequest, mapping *models.PortMapping) error {
	utils.Infof("TunnelManager: target client tunnel open, client_id=%d, tunnel_id=%s", conn.ClientID, req.TunnelID)

	// 1. ✅ 查找现有 Tunnel（必须已由源端创建）
	tm.tunnelsMu.Lock()
	tunnel := tm.tunnels[req.TunnelID]
	if tunnel == nil {
		tm.tunnelsMu.Unlock()
		utils.Warnf("TunnelManager: tunnel not found for target open, tunnel_id=%s", req.TunnelID)
		return errors.New("tunnel not found")
	}

	// 2. ✅ 关联目标连接
	if tunnel.TargetConn != nil {
		tm.tunnelsMu.Unlock()
		utils.Warnf("TunnelManager: tunnel target already set, tunnel_id=%s", req.TunnelID)
		return errors.New("tunnel target already set")
	}

	tunnel.TargetConn = conn
	tunnel.LastActiveAt = time.Now()
	tunnel.copyDone = make(chan struct{})
	tm.tunnelsMu.Unlock()

	utils.Infof("TunnelManager: tunnel target connected, tunnel_id=%s", req.TunnelID)

	// 3. 发送 TunnelOpenAck 给目标客户端
	if err := tm.sendTunnelOpenAck(conn, req.TunnelID, true, ""); err != nil {
		return err
	}

	// 4. ✅ 启动双向纯透传（本地转发）
	if tunnel.IsLocal {
		go tm.bidirectionalCopyLocal(tunnel)
	}
	// 跨节点转发由 BridgeManager 处理

	return nil
}

// sendTunnelOpenRequestToTarget 向目标客户端发送 TunnelOpenRequest 命令
func (tm *TunnelManager) sendTunnelOpenRequestToTarget(targetConn *session.ControlConnection, tunnelID string, mapping *models.PortMapping) error {
	// ✅ 构造命令体（包含压缩、加密配置）
	cmdBody := map[string]interface{}{
		"tunnel_id":   tunnelID,
		"mapping_id":  mapping.ID,
		"secret_key":  mapping.SecretKey,
		"target_host": mapping.TargetHost,
		"target_port": mapping.TargetPort,

		// ✅ 压缩、加密配置
		"enable_compression": mapping.Config.EnableCompression,
		"compression_level":  mapping.Config.CompressionLevel,
		"enable_encryption":  mapping.Config.EnableEncryption,
		"encryption_method":  mapping.Config.EncryptionMethod,
		"encryption_key":     mapping.Config.EncryptionKey,
	}

	cmdBodyJSON, _ := json.Marshal(cmdBody)

	cmdPkt := &packet.CommandPacket{
		CommandType: packet.TunnelOpenRequestCmd,
		CommandId:   fmt.Sprintf("tunnel-req-%s-%d", tunnelID, time.Now().UnixNano()),
		SenderId:    "server",
		ReceiverId:  fmt.Sprintf("%d", mapping.TargetClientID),
		CommandBody: string(cmdBodyJSON),
	}

	transferPkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmdPkt,
	}

	_, err := targetConn.Stream.WritePacket(transferPkt, false, 0)
	if err != nil {
		return fmt.Errorf("failed to write packet: %w", err)
	}

	utils.Infof("TunnelManager: TunnelOpenRequest sent to target client %d", mapping.TargetClientID)
	return nil
}

// sendTunnelOpenAck 发送 TunnelOpenAck 响应
func (tm *TunnelManager) sendTunnelOpenAck(conn *session.ClientConnection, tunnelID string, success bool, errorMsg string) error {
	ack := &packet.TunnelOpenAckResponse{
		TunnelID: tunnelID,
		Success:  success,
	}

	ackData, _ := json.Marshal(ack)
	ackPacket := &packet.TransferPacket{
		PacketType: packet.TunnelOpenAck,
		Payload:    ackData,
	}

	_, err := conn.Stream.WritePacket(ackPacket, false, 0)
	if err != nil {
		utils.Errorf("TunnelManager: failed to send tunnel open ack, tunnel_id=%s, error=%v", tunnelID, err)
		return fmt.Errorf("failed to send ack: %w", err)
	}

	utils.Debugf("TunnelManager: TunnelOpenAck sent, tunnel_id=%s, success=%v", tunnelID, success)
	return nil
}

// bidirectionalCopyLocal ✅ 双向纯透传（本地转发）
// SourceConn ↔ TargetConn（直接 Copy，不再组包）
func (tm *TunnelManager) bidirectionalCopyLocal(tunnel *Tunnel) {
	defer close(tunnel.copyDone)

	sourceConn := tunnel.SourceConn
	targetConn, ok := tunnel.TargetConn.(*session.ClientConnection)
	if !ok {
		utils.Errorf("TunnelManager: invalid target connection type for tunnel %s", tunnel.TunnelID)
		return
	}

	// ✅ 获取底层的 Reader/Writer
	sourceReader := sourceConn.Stream.GetReader()
	sourceWriter := sourceConn.Stream.GetWriter()
	targetReader := targetConn.Stream.GetReader()
	targetWriter := targetConn.Stream.GetWriter()

	utils.Infof("TunnelManager: starting bidirectional copy for tunnel %s", tunnel.TunnelID)

	// ✅ 使用适配器将 Reader/Writer 转换为 ReadWriteCloser
	sourceRWC := utils.NewReadWriteCloser(sourceReader, sourceWriter, func() error {
		// 关闭源连接的底层连接
		sourceConn.Stream.Close()
		return nil
	})
	targetRWC := utils.NewReadWriteCloser(targetReader, targetWriter, func() error {
		// 关闭目标连接的底层连接
		targetConn.Stream.Close()
		return nil
	})

	// TODO: 根据映射配置应用压缩、加密
	// 暂时使用无转换器版本

	// ✅ 使用通用双向拷贝函数，并记录统计信息
	result := utils.BidirectionalCopy(sourceRWC, targetRWC, &utils.BidirectionalCopyOptions{
		LogPrefix: fmt.Sprintf("TunnelManager[local][tunnel:%s]", tunnel.TunnelID),
		OnComplete: func(sent, received int64, err error) {
			atomic.AddUint64(&tunnel.BytesSent, uint64(sent))
			atomic.AddUint64(&tunnel.BytesReceived, uint64(received))
		},
	})

	utils.Infof("TunnelManager: tunnel %s closed (sent:%d, received:%d)",
		tunnel.TunnelID, result.BytesSent, result.BytesReceived)

	// ✅ 清理 Tunnel
	tm.tunnelsMu.Lock()
	delete(tm.tunnels, tunnel.TunnelID)
	tm.tunnelsMu.Unlock()
}

// cleanupLoop 清理空闲隧道
func (tm *TunnelManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tm.cleanupIdleTunnels()
		case <-tm.Ctx().Done():
			utils.Infof("TunnelManager: cleanup loop stopped")
			return
		}
	}
}

// cleanupIdleTunnels 清理空闲隧道
func (tm *TunnelManager) cleanupIdleTunnels() {
	now := time.Now()
	idleTimeout := 10 * time.Minute

	tm.tunnelsMu.Lock()
	defer tm.tunnelsMu.Unlock()

	for tunnelID, tunnel := range tm.tunnels {
		tunnel.mu.RLock()
		lastActive := tunnel.LastActiveAt
		tunnel.mu.RUnlock()

		if now.Sub(lastActive) > idleTimeout {
			utils.Infof("TunnelManager: cleaning up idle tunnel %s", tunnelID)
			tunnel.Close()
			delete(tm.tunnels, tunnelID)
		}
	}
}
