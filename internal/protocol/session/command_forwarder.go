// Package session 提供会话管理功能
// 本文件实现统一的跨节点命令转发器
package session

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session/connection"
)

// ============================================================================
// 统一命令转发器 - 自动处理本地/跨节点
// ============================================================================

// SendCommandToClient 向指定客户端发送命令
// 自动处理本地连接和跨节点转发，调用者无需关心客户端在哪个节点
//
// 参数:
//   - ctx: 上下文，用于超时控制
//   - targetClientID: 目标客户端ID
//   - cmd: 要发送的命令包
//   - timeout: 等待响应的超时时间
//
// 返回:
//   - *packet.CommandPacket: 响应命令包
//   - error: 错误信息
func (s *SessionManager) SendCommandToClient(
	ctx context.Context,
	targetClientID int64,
	cmd *packet.CommandPacket,
	timeout time.Duration,
) (*packet.CommandPacket, error) {
	// 1. 先尝试本地 control 连接
	targetConn := s.GetControlConnectionByClientID(targetClientID)
	if targetConn != nil && targetConn.Stream != nil {
		corelog.Debugf("CommandForwarder: sending command %s to local client %d",
			cmd.CommandId, targetClientID)
		return s.sendCommandLocal(ctx, targetConn, cmd, timeout)
	}

	// 2. 本地没有，走跨节点转发
	corelog.Debugf("CommandForwarder: client %d not on local node, trying cross-node",
		targetClientID)
	return s.sendCommandCrossNode(ctx, targetClientID, cmd, timeout)
}

// sendCommandLocal 发送命令到本地客户端
func (s *SessionManager) sendCommandLocal(
	ctx context.Context,
	targetConn *connection.ControlConnection,
	cmd *packet.CommandPacket,
	timeout time.Duration,
) (*packet.CommandPacket, error) {
	// 注册等待响应
	waitCh := s.commandResponseMgr.Register(cmd.CommandId)
	defer s.commandResponseMgr.Unregister(cmd.CommandId)

	// 构建并发送命令包
	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmd,
	}

	if _, err := targetConn.Stream.WritePacket(pkt, true, 0); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send command to client")
	}

	// 等待响应
	return s.commandResponseMgr.Wait(ctx, cmd.CommandId, waitCh, timeout)
}

// sendCommandCrossNode 跨节点发送命令
func (s *SessionManager) sendCommandCrossNode(
	ctx context.Context,
	targetClientID int64,
	cmd *packet.CommandPacket,
	timeout time.Duration,
) (*packet.CommandPacket, error) {
	// 1. 检查跨节点组件是否可用
	if s.connStateStore == nil || s.crossNodePool == nil {
		return nil, coreerrors.New(coreerrors.CodeUnavailable, "cross-node components not available")
	}

	// 2. 从 Redis 查找目标客户端所在节点
	targetNodeID, _, err := s.connStateStore.FindClientNode(ctx, targetClientID)
	if err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeNotFound, "target client %d not connected", targetClientID)
	}

	// 3. 如果目标节点是本节点但连接不存在，说明状态不一致
	if targetNodeID == s.nodeID {
		return nil, coreerrors.Newf(coreerrors.CodeInternal,
			"client %d state inconsistent (on local node but not found)", targetClientID)
	}

	corelog.Infof("CommandForwarder: forwarding command %s to node %s for client %d",
		cmd.CommandId, targetNodeID, targetClientID)

	// 4. 获取跨节点连接
	crossConn, err := s.crossNodePool.Get(ctx, targetNodeID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to get cross-node connection")
	}
	defer s.crossNodePool.Put(crossConn)

	// 5. 构建跨节点命令消息
	cmdMsg := &CommandMessage{
		CommandID:      cmd.CommandId,
		CommandType:    byte(cmd.CommandType),
		TargetClientID: targetClientID,
		SourceNodeID:   s.nodeID,
		SourceConnID:   "", // 用于直接响应时使用
		Payload:        []byte(cmd.CommandBody),
	}

	msgBody, err := json.Marshal(cmdMsg)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to marshal command message")
	}

	// 6. 发送命令请求帧
	tcpConn := crossConn.GetTCPConn()
	if tcpConn == nil {
		crossConn.MarkBroken()
		return nil, coreerrors.New(coreerrors.CodeNetworkError, "cross-node connection is nil")
	}

	var emptyTunnelID [16]byte
	if err := WriteFrame(tcpConn, emptyTunnelID, FrameTypeCommand, msgBody); err != nil {
		crossConn.MarkBroken()
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send cross-node command")
	}

	// 7. 等待响应
	tcpConn.SetReadDeadline(time.Now().Add(timeout))
	defer tcpConn.SetReadDeadline(time.Time{})

	_, frameType, respData, err := ReadFrame(tcpConn)
	if err != nil {
		crossConn.MarkBroken()
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read cross-node response")
	}

	if frameType != FrameTypeCommandResponse {
		crossConn.MarkBroken()
		return nil, coreerrors.Newf(coreerrors.CodeInvalidPacket,
			"unexpected response frame type: %d", frameType)
	}

	// 8. 解析响应
	var respMsg CommandResponseMessage
	if err := json.Unmarshal(respData, &respMsg); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to unmarshal response")
	}

	if !respMsg.Success {
		return nil, coreerrors.New(coreerrors.CodeInternal, respMsg.Error)
	}

	// 9. 构建响应命令包
	respCmd := &packet.CommandPacket{
		CommandType: packet.CommandType(respMsg.CommandType),
		CommandId:   respMsg.CommandID,
		CommandBody: string(respMsg.Payload),
	}

	return respCmd, nil
}

// ============================================================================
// 命令响应管理器 - 用于本地命令的请求/响应匹配
// ============================================================================

// CommandResponseManager 命令响应管理器
type CommandResponseManager struct {
	mu      sync.RWMutex
	waiters map[string]chan *packet.CommandPacket
}

// NewCommandResponseManager 创建命令响应管理器
func NewCommandResponseManager() *CommandResponseManager {
	return &CommandResponseManager{
		waiters: make(map[string]chan *packet.CommandPacket),
	}
}

// Register 注册等待响应
func (m *CommandResponseManager) Register(commandID string) chan *packet.CommandPacket {
	ch := make(chan *packet.CommandPacket, 1)
	m.mu.Lock()
	m.waiters[commandID] = ch
	m.mu.Unlock()
	return ch
}

// Unregister 取消注册
func (m *CommandResponseManager) Unregister(commandID string) {
	m.mu.Lock()
	delete(m.waiters, commandID)
	m.mu.Unlock()
}

// Deliver 投递响应
func (m *CommandResponseManager) Deliver(commandID string, resp *packet.CommandPacket) bool {
	m.mu.RLock()
	ch, exists := m.waiters[commandID]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	select {
	case ch <- resp:
		return true
	default:
		return false
	}
}

// Wait 等待响应
func (m *CommandResponseManager) Wait(
	ctx context.Context,
	commandID string,
	ch chan *packet.CommandPacket,
	timeout time.Duration,
) (*packet.CommandPacket, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case resp := <-ch:
		return resp, nil
	case <-timeoutCtx.Done():
		return nil, coreerrors.Newf(coreerrors.CodeTimeout,
			"timeout waiting for command response: %s", commandID)
	}
}

// ============================================================================
// SessionManager 初始化命令响应管理器
// ============================================================================

// initCommandResponseManager 初始化命令响应管理器
func (s *SessionManager) initCommandResponseManager() {
	if s.commandResponseMgr == nil {
		s.commandResponseMgr = NewCommandResponseManager()
	}
}

// DeliverCommandResponse 投递命令响应（供 readLoop 调用）
func (s *SessionManager) DeliverCommandResponse(commandID string, resp *packet.CommandPacket) bool {
	if s.commandResponseMgr == nil {
		return false
	}
	return s.commandResponseMgr.Deliver(commandID, resp)
}
