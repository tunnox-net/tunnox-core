package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// TunnelRole 定义隧道角色
type TunnelRole int

const (
	TunnelRoleListen TunnelRole = iota // 监听端发起的 tunnel
	TunnelRoleTarget                    // 目标端接收的 tunnel
)

func (r TunnelRole) String() string {
	switch r {
	case TunnelRoleListen:
		return "Listen"
	case TunnelRoleTarget:
		return "Target"
	default:
		return "Unknown"
	}
}

// CloseReason 定义关闭原因
type CloseReason int

const (
	CloseReasonNormal          CloseReason = iota // 正常关闭
	CloseReasonLocalClosed                        // 本地连接关闭
	CloseReasonPeerClosed                         // 对端关闭通知
	CloseReasonTimeout                            // 超时
	CloseReasonError                              // 错误
	CloseReasonContextCanceled                    // Context 取消
)

func (r CloseReason) String() string {
	switch r {
	case CloseReasonNormal:
		return "normal"
	case CloseReasonLocalClosed:
		return "local_closed"
	case CloseReasonPeerClosed:
		return "peer_closed"
	case CloseReasonTimeout:
		return "timeout"
	case CloseReasonError:
		return "error"
	case CloseReasonContextCanceled:
		return "context_canceled"
	default:
		return "unknown"
	}
}

// TunnelState 定义隧道状态
type TunnelState int32

const (
	TunnelStateConnecting TunnelState = iota
	TunnelStateConnected
	TunnelStateClosing
	TunnelStateClosed
)

// TunnelStats 隧道统计信息
type TunnelStats struct {
	BytesSent  int64
	BytesRecv  int64
	DurationMs int64
}

// ClientInterface 定义客户端接口（避免循环依赖）
type ClientInterface interface {
	SendTunnelCloseNotify(targetClientID int64, tunnelID, mappingID, reason string) error
}

// TunnelConfig 隧道配置
type TunnelConfig struct {
	ID           string
	MappingID    string
	Role         TunnelRole
	LocalConn    io.ReadWriteCloser
	TunnelConn   net.Conn
	TunnelRWC    io.ReadWriteCloser
	TargetClient int64
	Manager      TunnelManager
	Client       ClientInterface
	OnClosed     func(reason CloseReason, err error)
}

// Tunnel 隧道结构
type Tunnel struct {
	dispose.Dispose

	// 标识
	id        string
	mappingID string
	role      TunnelRole

	// 连接
	localConn  io.ReadWriteCloser
	tunnelConn net.Conn
	tunnelRWC  io.ReadWriteCloser

	// 对端信息
	targetClient int64

	// 状态
	state     atomic.Int32
	startTime time.Time

	// 统计
	bytesSent atomic.Int64
	bytesRecv atomic.Int64

	// 活动信号
	activityChan chan struct{}

	// 回调
	onClosed func(reason CloseReason, err error)

	// 管理器引用
	manager TunnelManager
	client  ClientInterface
}

// NewTunnel 创建新的隧道
func NewTunnel(config *TunnelConfig) *Tunnel {
	t := &Tunnel{
		id:           config.ID,
		mappingID:    config.MappingID,
		role:         config.Role,
		localConn:    config.LocalConn,
		tunnelConn:   config.TunnelConn,
		tunnelRWC:    config.TunnelRWC,
		targetClient: config.TargetClient,
		manager:      config.Manager,
		client:       config.Client,
		onClosed:     config.OnClosed,
		activityChan: make(chan struct{}, 1),
		startTime:    time.Now(),
	}

	t.state.Store(int32(TunnelStateConnecting))

	return t
}

// Start 启动隧道
func (t *Tunnel) Start() error {
	// 设置 context（从 manager 继承）
	if t.manager == nil {
		return fmt.Errorf("tunnel manager is nil")
	}

	t.SetCtx(t.manager.Ctx(), t.onClose)

	// 更新状态
	if !t.state.CompareAndSwap(int32(TunnelStateConnecting), int32(TunnelStateConnected)) {
		return fmt.Errorf("invalid state transition")
	}

	corelog.Infof("Tunnel[%s][%s]: starting, role=%s", t.role, t.id, t.role)

	// 启动监控 goroutines
	go t.monitorPeerNotification()
	go t.monitorTimeout()

	// 启动双向数据复制
	go t.runDataCopy()

	return nil
}

// monitorPeerNotification 监控对端关闭通知
func (t *Tunnel) monitorPeerNotification() {
	<-t.Ctx().Done()

	// Context 被取消，检查原因
	if t.Ctx().Err() == context.Canceled {
		// 可能是收到了对端关闭通知
		corelog.Debugf("Tunnel[%s][%s]: context canceled", t.role, t.id)
	}
}

// monitorTimeout 监控超时
func (t *Tunnel) monitorTimeout() {
	// 空闲超时：5分钟没有数据传输
	idleTimeout := 5 * time.Minute
	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()

	lastActivity := time.Now()

	for {
		select {
		case <-t.Ctx().Done():
			return
		case <-timer.C:
			if time.Since(lastActivity) >= idleTimeout {
				corelog.Warnf("Tunnel[%s][%s]: idle timeout", t.role, t.id)
				t.Close(CloseReasonTimeout, fmt.Errorf("idle timeout"))
				return
			}
			timer.Reset(idleTimeout)
		case <-t.activityChan:
			lastActivity = time.Now()
			timer.Reset(idleTimeout)
		}
	}
}

// runDataCopy 运行双向数据复制
func (t *Tunnel) runDataCopy() {
	startTime := time.Now()

	options := &utils.BidirectionalCopyOptions{
		LogPrefix: fmt.Sprintf("Tunnel[%s][%s]", t.role, t.id),
	}

	result := utils.BidirectionalCopy(t.localConn, t.tunnelRWC, options)

	// 更新流量统计
	t.bytesSent.Add(result.BytesSent)
	t.bytesRecv.Add(result.BytesReceived)

	// 根据复制结果判断关闭原因
	var reason CloseReason
	var err error

	if result.SendError != nil || result.ReceiveError != nil {
		reason = CloseReasonError
		err = result.SendError
		if err == nil {
			err = result.ReceiveError
		}

		// 检查是否是正常的 EOF
		if err == io.EOF || (err != nil && err.Error() == "EOF") {
			reason = CloseReasonLocalClosed
		} else {
			corelog.Warnf("Tunnel[%s][%s]: data copy error - sendErr=%v, recvErr=%v",
				t.role, t.id, result.SendError, result.ReceiveError)
		}
	} else {
		reason = CloseReasonNormal
	}

	elapsed := time.Since(startTime)
	corelog.Infof("Tunnel[%s][%s]: data copy finished, reason=%s, sent=%d, recv=%d, duration=%v",
		t.role, t.id, reason, result.BytesSent, result.BytesReceived, elapsed)

	t.Close(reason, err)
}

// Close 关闭隧道
func (t *Tunnel) Close(reason CloseReason, err error) error {
	// CAS 更新状态
	currentState := TunnelState(t.state.Load())
	if currentState == TunnelStateClosing || currentState == TunnelStateClosed {
		return nil // 已经在关闭或已关闭
	}

	if !t.state.CompareAndSwap(int32(TunnelStateConnected), int32(TunnelStateClosing)) {
		// 可能是从 Connecting 状态直接关闭
		t.state.Store(int32(TunnelStateClosing))
	}

	duration := time.Since(t.startTime).Milliseconds()

	corelog.Infof("Tunnel[%s][%s]: closing, reason=%s, error=%v, duration=%dms, sent=%d, recv=%d",
		t.role, t.id, reason, err, duration, t.bytesSent.Load(), t.bytesRecv.Load())

	// 取消 context（触发所有监控 goroutine 退出）
	t.Dispose.Close()

	// 关闭连接（忽略错误，因为可能已经关闭）
	if t.localConn != nil {
		_ = t.localConn.Close()
	}
	if t.tunnelRWC != nil {
		_ = t.tunnelRWC.Close()
	}

	// 发送关闭通知给对端（如果需要）
	if t.shouldNotifyPeer(reason) {
		t.sendCloseNotification(reason)
	}

	// 从管理器注销
	if t.manager != nil {
		t.manager.UnregisterTunnel(t.id)
	}

	// 调用回调
	if t.onClosed != nil {
		t.onClosed(reason, err)
	}

	// 更新最终状态
	t.state.Store(int32(TunnelStateClosed))

	return nil
}

// shouldNotifyPeer 判断是否需要通知对端
func (t *Tunnel) shouldNotifyPeer(reason CloseReason) bool {
	switch reason {
	case CloseReasonPeerClosed:
		return false // 对端已经知道了
	case CloseReasonContextCanceled:
		return false // Context 取消可能就是因为收到了对端通知
	default:
		return true
	}
}

// sendCloseNotification 发送关闭通知
func (t *Tunnel) sendCloseNotification(reason CloseReason) {
	if t.client == nil {
		return
	}

	// ListenClient 需要通知 TargetClient
	// TargetClient 需要通知 ListenClient（通过 server 中转）
	if t.role == TunnelRoleListen && t.targetClient > 0 {
		// 异步发送，不阻塞关闭流程
		go func() {
			err := t.client.SendTunnelCloseNotify(
				t.targetClient,
				t.id,
				t.mappingID,
				reason.String(),
			)
			if err != nil {
				corelog.Warnf("Tunnel[%s][%s]: failed to send close notify: %v", t.role, t.id, err)
			}
		}()
	}
	// TargetClient 的关闭通知由 server 自动处理
}

// GetID 获取隧道 ID
func (t *Tunnel) GetID() string {
	return t.id
}

// GetRole 获取角色
func (t *Tunnel) GetRole() TunnelRole {
	return t.role
}

// GetState 获取状态
func (t *Tunnel) GetState() TunnelState {
	return TunnelState(t.state.Load())
}

// GetStats 获取统计信息
func (t *Tunnel) GetStats() *TunnelStats {
	return &TunnelStats{
		BytesSent:  t.bytesSent.Load(),
		BytesRecv:  t.bytesRecv.Load(),
		DurationMs: time.Since(t.startTime).Milliseconds(),
	}
}

// onClose dispose 回调
func (t *Tunnel) onClose() error {
	corelog.Debugf("Tunnel[%s][%s]: dispose onClose called", t.role, t.id)
	return nil
}

// NotifyPeerClosed 接收对端关闭通知（由 TunnelManager 调用）
func (t *Tunnel) NotifyPeerClosed(reason string, stats *TunnelStats) {
	corelog.Infof("Tunnel[%s][%s]: received peer close notification, reason=%s", t.role, t.id, reason)

	// 触发关闭
	t.Close(CloseReasonPeerClosed, nil)
}

// TunnelClosedPayload 关闭通知载荷（与 packet 包对齐）
type TunnelClosedPayload struct {
	TunnelID   string
	MappingID  string
	Reason     string
	BytesSent  int64
	BytesRecv  int64
	DurationMs int64
	ClosedAt   int64
}

// FromPacketPayload 从 packet.TunnelClosedPayload 转换
func (p *TunnelClosedPayload) FromPacketPayload(pp *packet.TunnelClosedPayload) {
	p.TunnelID = pp.TunnelID
	p.MappingID = pp.MappingID
	p.Reason = pp.Reason
	p.BytesSent = pp.BytesSent
	p.BytesRecv = pp.BytesRecv
	p.DurationMs = pp.Duration
	p.ClosedAt = pp.ClosedAt
}
