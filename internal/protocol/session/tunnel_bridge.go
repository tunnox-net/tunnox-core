package session

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"

	"golang.org/x/time/rate"
)

// DataForwarder 数据转发接口（依赖倒置：不依赖具体协议）
// 抽象了不同协议的数据转发能力
type DataForwarder interface {
	io.ReadWriteCloser
}

// StreamDataForwarder 流数据转发器接口（用于 HTTP 长轮询等协议）
type StreamDataForwarder interface {
	ReadExact(length int) ([]byte, error)
	ReadAvailable(maxLength int) ([]byte, error) // 读取可用数据（不等待完整长度）
	WriteExact(data []byte) error
	Close()
	GetConnectionID() string // 获取连接ID（用于调试）
}

// checkStreamDataForwarder 检查 stream 是否实现了 StreamDataForwarder 接口
// 使用接口查询，避免跨包类型断言问题
func checkStreamDataForwarder(stream stream.PackageStreamer) StreamDataForwarder {
	// 使用接口查询检查是否有 ReadExact、ReadAvailable 和 WriteExact 方法
	type readExact interface {
		ReadExact(length int) ([]byte, error)
	}
	type readAvailable interface {
		ReadAvailable(maxLength int) ([]byte, error)
	}
	type writeExact interface {
		WriteExact(data []byte) error
	}
	type closer interface {
		Close()
	}

	if r, ok := stream.(readExact); ok {
		utils.Infof("checkStreamDataForwarder: detected ReadExact method")
		if w, ok := stream.(writeExact); ok {
			utils.Infof("checkStreamDataForwarder: detected WriteExact method")
			if c, ok := stream.(closer); ok {
				utils.Infof("checkStreamDataForwarder: detected Close method")
				// 检查是否有 ReadAvailable 方法
				var ra readAvailable
				if streamRA, ok := stream.(readAvailable); ok {
					ra = streamRA
					utils.Infof("checkStreamDataForwarder: detected ReadAvailable method in stream")
				} else {
					utils.Warnf("checkStreamDataForwarder: ReadAvailable method not found in stream, will fallback to ReadExact")
				}
				// 检查是否有 GetConnectionID 方法
				type getConnID interface {
					GetConnectionID() string
				}
				var gci getConnID
				if streamGCI, ok := stream.(getConnID); ok {
					gci = streamGCI
					connID := streamGCI.GetConnectionID()
					utils.Infof("checkStreamDataForwarder: detected GetConnectionID method in stream, connID=%s", connID)
				} else {
					utils.Warnf("checkStreamDataForwarder: GetConnectionID method not found in stream")
				}
				// 创建一个包装器，实现 StreamDataForwarder 接口
				utils.Infof("checkStreamDataForwarder: creating streamDataForwarderWrapper, hasReadAvailable=%v, hasGetConnID=%v", ra != nil, gci != nil)
				return &streamDataForwarderWrapper{
					readExact:     r,
					readAvailable: ra,
					writeExact:    w,
					closer:        c,
					getConnID:     gci,
				}
			} else {
				utils.Warnf("checkStreamDataForwarder: Close method not found")
			}
		} else {
			utils.Warnf("checkStreamDataForwarder: WriteExact method not found")
		}
	} else {
		utils.Warnf("checkStreamDataForwarder: ReadExact method not found")
	}
	utils.Warnf("checkStreamDataForwarder: returning nil (stream does not implement required methods)")
	return nil
}

// streamDataForwarderWrapper 包装实现了 ReadExact/ReadAvailable/WriteExact/Close 的对象
type streamDataForwarderWrapper struct {
	readExact interface {
		ReadExact(length int) ([]byte, error)
	}
	readAvailable interface {
		ReadAvailable(maxLength int) ([]byte, error)
	}
	writeExact interface{ WriteExact(data []byte) error }
	closer     interface{ Close() }
	getConnID  interface{ GetConnectionID() string } // 可选的 GetConnectionID 方法
}

func (w *streamDataForwarderWrapper) ReadExact(length int) ([]byte, error) {
	return w.readExact.ReadExact(length)
}

func (w *streamDataForwarderWrapper) ReadAvailable(maxLength int) ([]byte, error) {
	connID := "unknown"
	if w.getConnID != nil {
		connID = w.getConnID.GetConnectionID()
	}
	if w.readAvailable != nil {
		utils.Infof("streamDataForwarderWrapper[connID=%s]: ReadAvailable calling underlying ReadAvailable, maxLength=%d", connID, maxLength)
		data, err := w.readAvailable.ReadAvailable(maxLength)
		utils.Infof("streamDataForwarderWrapper[connID=%s]: ReadAvailable returned, data len=%d, err=%v", connID, len(data), err)
		return data, err
	}
	// 如果没有 ReadAvailable，回退到 ReadExact（但只请求较小的长度）
	utils.Warnf("streamDataForwarderWrapper[connID=%s]: ReadAvailable not available, falling back to ReadExact, maxLength=%d", connID, maxLength)
	if maxLength > 256 {
		maxLength = 256
	}
	data, err := w.readExact.ReadExact(maxLength)
	utils.Infof("streamDataForwarderWrapper[connID=%s]: ReadExact (fallback) returned, data len=%d, err=%v", connID, len(data), err)
	return data, err
}

func (w *streamDataForwarderWrapper) WriteExact(data []byte) error {
	return w.writeExact.WriteExact(data)
}

func (w *streamDataForwarderWrapper) Close() {
	w.closer.Close()
}

func (w *streamDataForwarderWrapper) GetConnectionID() string {
	if w.getConnID != nil {
		return w.getConnID.GetConnectionID()
	}
	return "unknown"
}

// streamDataForwarderAdapter 将 StreamDataForwarder 适配为 DataForwarder
type streamDataForwarderAdapter struct {
	stream StreamDataForwarder
	buf    []byte
	bufMu  sync.Mutex
	closed bool
}

func (a *streamDataForwarderAdapter) Read(p []byte) (int, error) {
	a.bufMu.Lock()
	defer a.bufMu.Unlock()

	if a.closed {
		return 0, io.EOF
	}

	// 如果缓冲区有数据，先返回缓冲区数据
	if len(a.buf) > 0 {
		n := copy(p, a.buf)
		a.buf = a.buf[n:]
		utils.Debugf("streamDataForwarderAdapter: Read from buffer, n=%d, remaining=%d", n, len(a.buf))
		return n, nil
	}

	// 从流读取数据：使用 ReadAvailable 读取可用数据，避免长时间阻塞
	// ReadAvailable 会返回可用数据，不等待完整长度，适合 io.Reader 接口
	// 对于大数据包，需要支持更大的 maxLength 以确保数据完整性
	maxLength := len(p)
	if maxLength > 32*1024 {
		maxLength = 32 * 1024 // 限制最大读取长度为 32KB，平衡性能和内存使用
	}
	if maxLength == 0 {
		return 0, nil
	}

	data, err := a.stream.ReadAvailable(maxLength)
	if err != nil {
		if err == io.EOF {
			a.closed = true
		}
		// 如果有部分数据，返回它
		if len(data) > 0 {
			n := copy(p, data)
			return n, nil
		}
		return 0, err
	}

	if len(data) == 0 {
		// 没有数据，返回0但不返回错误（表示暂时没有数据）
		// 注意：io.Reader 的 Read 方法返回 0, nil 表示暂时没有数据，调用者应该重试
		return 0, nil
	}

	// 返回可用数据
	n := copy(p, data)
	if n < len(data) {
		// 如果读取的数据比请求的多，将多余的数据放入缓冲区
		a.buf = append(a.buf, data[n:]...)
	}
	return n, nil
}

func (a *streamDataForwarderAdapter) Write(p []byte) (int, error) {
	if a.closed {
		return 0, io.ErrClosedPipe
	}

	if err := a.stream.WriteExact(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (a *streamDataForwarderAdapter) Close() error {
	a.bufMu.Lock()
	defer a.bufMu.Unlock()

	if a.closed {
		return nil
	}
	a.closed = true
	a.stream.Close()
	return nil
}

// createDataForwarder 创建数据转发器（通过接口抽象，不依赖具体协议）
// 优先使用 net.Conn，如果没有则尝试从 Stream 创建适配器
func createDataForwarder(conn net.Conn, stream stream.PackageStreamer) DataForwarder {
	utils.Infof("createDataForwarder: called, conn=%v, stream=%v, stream type=%T", conn != nil, stream != nil, stream)
	if conn != nil {
		utils.Infof("createDataForwarder: using net.Conn, remoteAddr=%s", conn.RemoteAddr())
		return conn // net.Conn 实现了 io.ReadWriteCloser
	}
	if stream != nil {
		reader := stream.GetReader()
		writer := stream.GetWriter()
		utils.Infof("createDataForwarder: stream has GetReader=%v, GetWriter=%v", reader != nil, writer != nil)
		if reader != nil && writer != nil {
			// 使用 Stream 的 Reader/Writer 创建适配器
			utils.Infof("createDataForwarder: using Stream Reader/Writer adapter")
			return utils.NewReadWriteCloser(reader, writer, func() error {
				stream.Close()
				return nil
			})
		}
		// 如果 GetReader/GetWriter 返回 nil，尝试使用 ReadExact/WriteExact（HTTP 长轮询）
		// 使用接口查询，检查是否有 ReadExact 和 WriteExact 方法
		utils.Infof("createDataForwarder: checking stream for StreamDataForwarder, stream type=%T", stream)
		if streamForwarder := checkStreamDataForwarder(stream); streamForwarder != nil {
			connID := "unknown"
			if connIDGetter, ok := streamForwarder.(interface{ GetConnectionID() string }); ok {
				connID = connIDGetter.GetConnectionID()
			}
			utils.Infof("createDataForwarder: StreamDataForwarder detected, creating adapter, connID=%s", connID)
			return &streamDataForwarderAdapter{stream: streamForwarder}
		}
		utils.Warnf("createDataForwarder: StreamDataForwarder not detected, stream type=%T", stream)
	}
	// 如果都没有，返回 nil（表示该协议不支持桥接）
	utils.Warnf("createDataForwarder: returning nil (no suitable forwarder found)")
	return nil
}

// TunnelBridge 隧道桥接器
// 负责在两个隧道连接之间进行数据桥接（源端客户端 <-> 目标端客户端）
//
// 商业特性支持：
// - 流量统计：统计实际传输的字节数（已加密/压缩后）
// - 带宽限制：使用 Token Bucket 限制传输速率
// - 连接监控：记录活跃隧道和传输状态
type TunnelBridge struct {
	*dispose.ManagerBase

	tunnelID  string
	mappingID string // 映射ID（用于流量统计）

	// 统一接口（推荐使用）
	sourceTunnelConn TunnelConnectionInterface // 源端隧道连接（统一接口）
	targetTunnelConn TunnelConnectionInterface // 目标端隧道连接（统一接口）
	tunnelConnMu     sync.RWMutex              // 保护隧道连接的读写

	// 向后兼容（逐步迁移）
	sourceConn      net.Conn // 源端客户端的隧道连接
	sourceStream    stream.PackageStreamer
	sourceForwarder DataForwarder // 源端数据转发器（接口抽象）
	sourceConnMu    sync.RWMutex  // 保护 sourceConn 的读写
	targetConn      net.Conn      // 目标端客户端的隧道连接
	targetStream    stream.PackageStreamer
	targetForwarder DataForwarder // 目标端数据转发器（接口抽象）

	ready chan struct{} // 用于通知目标端连接已建立

	// 商业特性
	rateLimiter          *rate.Limiter   // 带宽限制器
	bytesSent            atomic.Int64    // 源端→目标端字节数
	bytesReceived        atomic.Int64    // 目标端→源端字节数
	lastReportedSent     atomic.Int64    // 上次上报的发送字节数
	lastReportedReceived atomic.Int64    // 上次上报的接收字节数
	cloudControl         CloudControlAPI // 用于上报流量统计
}

// TunnelBridgeConfig 隧道桥接器配置
type TunnelBridgeConfig struct {
	TunnelID  string
	MappingID string

	// 统一接口（推荐）
	SourceTunnelConn TunnelConnectionInterface

	// 向后兼容
	SourceConn   net.Conn
	SourceStream stream.PackageStreamer

	BandwidthLimit int64           // 字节/秒，0表示不限制
	CloudControl   CloudControlAPI // 用于上报流量统计（可选）
}

// NewTunnelBridge 创建隧道桥接器
func NewTunnelBridge(parentCtx context.Context, config *TunnelBridgeConfig) *TunnelBridge {
	bridge := &TunnelBridge{
		ManagerBase:  dispose.NewManager(fmt.Sprintf("TunnelBridge-%s", config.TunnelID), parentCtx),
		tunnelID:     config.TunnelID,
		mappingID:    config.MappingID,
		cloudControl: config.CloudControl,
		ready:        make(chan struct{}),
	}

	// 优先使用统一接口
	if config.SourceTunnelConn != nil {
		bridge.sourceTunnelConn = config.SourceTunnelConn
		bridge.sourceConn = config.SourceTunnelConn.GetNetConn()
		bridge.sourceStream = config.SourceTunnelConn.GetStream()
	} else {
		// 向后兼容：从 net.Conn + stream 创建统一接口
		bridge.sourceConn = config.SourceConn
		bridge.sourceStream = config.SourceStream
		if config.SourceConn != nil || config.SourceStream != nil {
			// 提取连接信息
			connID := ""
			if config.SourceConn != nil {
				connID = config.SourceConn.RemoteAddr().String()
			}
			clientID := extractClientID(config.SourceStream, config.SourceConn)
			bridge.sourceTunnelConn = CreateTunnelConnection(
				connID,
				config.SourceConn,
				config.SourceStream,
				clientID,
				config.MappingID,
				config.TunnelID,
			)
		}
	}

	// 创建数据转发器
	bridge.sourceForwarder = createDataForwarder(bridge.sourceConn, bridge.sourceStream)

	// 配置带宽限制
	if config.BandwidthLimit > 0 {
		bridge.rateLimiter = rate.NewLimiter(rate.Limit(config.BandwidthLimit), int(config.BandwidthLimit*2))
		utils.Infof("TunnelBridge[%s]: bandwidth limit set to %d bytes/sec", config.TunnelID, config.BandwidthLimit)
	}

	// 注册清理处理器
	bridge.AddCleanHandler(func() error {
		utils.Infof("TunnelBridge[%s]: cleaning up resources", config.TunnelID)

		// 上报最终流量统计
		bridge.reportTrafficStats()

		var errs []error

		// 关闭统一接口连接
		if bridge.sourceTunnelConn != nil {
			if err := bridge.sourceTunnelConn.Close(); err != nil {
				errs = append(errs, fmt.Errorf("source tunnel conn close error: %w", err))
			}
		}
		if bridge.targetTunnelConn != nil {
			if err := bridge.targetTunnelConn.Close(); err != nil {
				errs = append(errs, fmt.Errorf("target tunnel conn close error: %w", err))
			}
		}

		// 向后兼容：关闭旧接口
		if bridge.sourceStream != nil && bridge.sourceTunnelConn == nil {
			bridge.sourceStream.Close()
		}
		if bridge.targetStream != nil && bridge.targetTunnelConn == nil {
			bridge.targetStream.Close()
		}
		if bridge.sourceConn != nil && bridge.sourceTunnelConn == nil {
			if err := bridge.sourceConn.Close(); err != nil {
				errs = append(errs, fmt.Errorf("source conn close error: %w", err))
			}
		}
		if bridge.targetConn != nil && bridge.targetTunnelConn == nil {
			if err := bridge.targetConn.Close(); err != nil {
				errs = append(errs, fmt.Errorf("target conn close error: %w", err))
			}
		}

		if len(errs) > 0 {
			return fmt.Errorf("tunnel bridge cleanup errors: %v", errs)
		}
		return nil
	})

	return bridge
}

// SetTargetConnection 设置目标端连接（统一接口）
func (b *TunnelBridge) SetTargetConnection(conn TunnelConnectionInterface) {
	b.tunnelConnMu.Lock()
	b.targetTunnelConn = conn
	if conn != nil {
		b.targetConn = conn.GetNetConn()
		b.targetStream = conn.GetStream()
		b.targetForwarder = createDataForwarder(b.targetConn, b.targetStream)
	}
	b.tunnelConnMu.Unlock()
	close(b.ready)
}

// SetTargetConnectionLegacy 设置目标端连接（向后兼容）
func (b *TunnelBridge) SetTargetConnectionLegacy(targetConn net.Conn, targetStream stream.PackageStreamer) {
	b.targetConn = targetConn
	b.targetStream = targetStream
	b.targetForwarder = createDataForwarder(targetConn, targetStream)

	// 创建统一接口
	if targetConn != nil || targetStream != nil {
		connID := ""
		if targetConn != nil {
			connID = targetConn.RemoteAddr().String()
		}
		clientID := extractClientID(targetStream, targetConn)
		b.tunnelConnMu.Lock()
		b.targetTunnelConn = CreateTunnelConnection(
			connID,
			targetConn,
			targetStream,
			clientID,
			b.mappingID,
			b.tunnelID,
		)
		b.tunnelConnMu.Unlock()
	}

	close(b.ready)
}

// SetSourceConnection 设置源端连接（统一接口）
func (b *TunnelBridge) SetSourceConnection(conn TunnelConnectionInterface) {
	b.tunnelConnMu.Lock()
	oldConn := b.sourceTunnelConn
	b.sourceTunnelConn = conn
	oldForwarder := b.sourceForwarder
	if conn != nil {
		b.sourceConn = conn.GetNetConn()
		b.sourceStream = conn.GetStream()
		connID := "unknown"
		if conn.GetStream() != nil {
			if streamConn, ok := conn.GetStream().(interface{ GetConnectionID() string }); ok {
				connID = streamConn.GetConnectionID()
			}
		}
		utils.Infof("TunnelBridge[%s]: SetSourceConnection creating forwarder, connID=%s, hasNetConn=%v, hasStream=%v", b.tunnelID, connID, b.sourceConn != nil, b.sourceStream != nil)
		b.sourceForwarder = createDataForwarder(b.sourceConn, b.sourceStream)
		utils.Infof("TunnelBridge[%s]: SetSourceConnection forwarder created, forwarder=%v, connID=%s", b.tunnelID, b.sourceForwarder != nil, connID)
	} else {
		b.sourceForwarder = nil
		utils.Infof("TunnelBridge[%s]: SetSourceConnection clearing connection", b.tunnelID)
	}
	b.tunnelConnMu.Unlock()
	utils.Infof("TunnelBridge[%s]: updated sourceConn (unified), mappingID=%s, oldConn=%v, newConn=%v, oldForwarder=%v, newForwarder=%v",
		b.tunnelID, b.mappingID, oldConn != nil, conn != nil, oldForwarder != nil, b.sourceForwarder != nil)
}

// SetSourceConnectionLegacy 设置源端连接（向后兼容）
func (b *TunnelBridge) SetSourceConnectionLegacy(sourceConn net.Conn, sourceStream stream.PackageStreamer) {
	b.sourceConnMu.Lock()
	oldConn := b.sourceConn
	b.sourceConn = sourceConn
	b.sourceForwarder = createDataForwarder(sourceConn, sourceStream)
	b.sourceConnMu.Unlock()
	if sourceStream != nil {
		b.sourceStream = sourceStream
	}

	// 创建统一接口
	if sourceConn != nil || sourceStream != nil {
		connID := ""
		if sourceConn != nil {
			connID = sourceConn.RemoteAddr().String()
		}
		clientID := extractClientID(sourceStream, sourceConn)
		b.tunnelConnMu.Lock()
		b.sourceTunnelConn = CreateTunnelConnection(
			connID,
			sourceConn,
			sourceStream,
			clientID,
			b.mappingID,
			b.tunnelID,
		)
		b.tunnelConnMu.Unlock()
	}

	utils.Infof("TunnelBridge[%s]: updated sourceConn (legacy), mappingID=%s, oldConn=%v, newConn=%v, hasForwarder=%v",
		b.tunnelID, b.mappingID, oldConn, sourceConn, b.sourceForwarder != nil)
}

// getSourceConn 获取源端连接（线程安全）
func (b *TunnelBridge) getSourceConn() net.Conn {
	b.sourceConnMu.RLock()
	defer b.sourceConnMu.RUnlock()
	return b.sourceConn
}

// Start 启动桥接
func (b *TunnelBridge) Start() error {
	// 等待目标端连接建立（超时30秒）
	select {
	case <-b.ready:
		utils.Infof("TunnelBridge[%s]: target connection established, starting bridge", b.tunnelID)
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for target connection")
	case <-b.Ctx().Done():
		return fmt.Errorf("bridge cancelled before target connection")
	}

	// 检查数据转发器是否可用（通过接口抽象，不依赖具体协议）
	if b.sourceForwarder == nil {
		// 尝试重新创建（可能是在 SetTargetConnection 之后才设置 source）
		utils.Infof("TunnelBridge[%s]: recreating sourceForwarder, sourceConn=%v, sourceStream=%v", b.tunnelID, b.sourceConn != nil, b.sourceStream != nil)
		b.sourceForwarder = createDataForwarder(b.sourceConn, b.sourceStream)
	}
	if b.targetForwarder == nil {
		// 尝试重新创建（可能是在 SetSourceConnection 之后才设置 target）
		utils.Infof("TunnelBridge[%s]: recreating targetForwarder, targetConn=%v, targetStream=%v", b.tunnelID, b.targetConn != nil, b.targetStream != nil)
		b.targetForwarder = createDataForwarder(b.targetConn, b.targetStream)
	}

	// 如果源端或目标端没有数据转发器，说明该协议不支持桥接（如 HTTP 长轮询）
	// 数据已经通过协议本身传输，只需要管理连接生命周期
	if b.sourceForwarder == nil || b.targetForwarder == nil {
		utils.Infof("TunnelBridge[%s]: connection does not support data forwarding (sourceForwarder=%v, targetForwarder=%v), bridge only manages connection lifecycle",
			b.tunnelID, b.sourceForwarder != nil, b.targetForwarder != nil)
		if b.cloudControl != nil && b.mappingID != "" {
			go b.periodicTrafficReport()
		}
		return nil
	}

	// ✅ 服务端是透明桥接，直接使用原始net.Conn转发（不解压不解密）
	// 压缩/加密由客户端两端处理，服务端只负责纯转发
	utils.Infof("TunnelBridge[%s]: bridge started, transparent forwarding (no compression/encryption on server)", b.tunnelID)

	// 启动双向数据转发（带流量统计和限速）
	// 源端 -> 目标端（带限速和统计）
	// ✅ 使用接口抽象，支持不同协议
	go func() {
		for {
			b.sourceConnMu.RLock()
			sourceForwarder := b.sourceForwarder
			b.sourceConnMu.RUnlock()

			if sourceForwarder == nil {
				utils.Warnf("TunnelBridge[%s]: sourceForwarder is nil, waiting...", b.tunnelID)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			utils.Infof("TunnelBridge[%s]: starting source->target copy", b.tunnelID)
			written := b.copyWithControl(b.targetForwarder, sourceForwarder, "source->target", &b.bytesSent)
			utils.Infof("TunnelBridge[%s]: source->target copy finished, %d bytes", b.tunnelID, written)

			// 检查连接是否更新
			b.sourceConnMu.RLock()
			newSourceForwarder := b.sourceForwarder
			b.sourceConnMu.RUnlock()

			if newSourceForwarder == nil {
				utils.Infof("TunnelBridge[%s]: sourceForwarder is nil, exiting", b.tunnelID)
				break
			}
			if newSourceForwarder == sourceForwarder {
				utils.Infof("TunnelBridge[%s]: sourceForwarder unchanged, exiting", b.tunnelID)
				break
			}
			utils.Infof("TunnelBridge[%s]: sourceForwarder updated, continuing with new connection", b.tunnelID)
		}
	}()

	// 目标端 -> 源端（带限速和统计）
	// ✅ 使用接口抽象，支持不同协议
	go func() {
		// 创建一个包装器，每次写入时都获取最新的 sourceForwarder
		dynamicWriter := &dynamicSourceWriter{bridge: b}
		written := b.copyWithControl(dynamicWriter, b.targetForwarder, "target->source", &b.bytesReceived)
		utils.Infof("TunnelBridge[%s]: target->source finished, %d bytes", b.tunnelID, written)
	}()

	// 启动定期流量统计上报（每30秒）
	if b.cloudControl != nil && b.mappingID != "" {
		go b.periodicTrafficReport()
	}

	return nil
}

// periodicTrafficReport 定期上报流量统计
func (b *TunnelBridge) periodicTrafficReport() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.reportTrafficStats()
		case <-b.Ctx().Done():
			// 最终上报
			b.reportTrafficStats()
			return
		}
	}
}

// copyWithControl 带流量统计和限速的数据拷贝
func (b *TunnelBridge) copyWithControl(dst io.Writer, src io.Reader, direction string, counter *atomic.Int64) int64 {
	buf := make([]byte, 32*1024) // 32KB buffer
	var total int64

	for {
		// 检查是否已取消
		select {
		case <-b.Ctx().Done():
			utils.Debugf("TunnelBridge[%s]: %s cancelled", b.tunnelID, direction)
			return total
		default:
		}

		// 从源端读取
		nr, err := src.Read(buf)
		if nr > 0 {
			// 应用限速（如果启用）
			if b.rateLimiter != nil {
				// 使用 bridge 的 context 进行限速等待
				if err := b.rateLimiter.WaitN(b.Ctx(), nr); err != nil {
					utils.Errorf("TunnelBridge[%s]: %s rate limit error: %v", b.tunnelID, direction, err)
					break
				}
			}

			// 写入目标端
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				total += int64(nw)
				counter.Add(int64(nw)) // 更新流量统计
			}
			if ew != nil {
				if ew != io.EOF {
					utils.Debugf("TunnelBridge[%s]: %s write error: %v", b.tunnelID, direction, ew)
				}
				break
			}
			if nr != nw {
				utils.Errorf("TunnelBridge[%s]: %s short write", b.tunnelID, direction)
				break
			}
		}
		if err != nil {
			// ✅ UDP 连接的超时错误是临时错误，不应该导致连接关闭
			if netErr, ok := err.(interface {
				Timeout() bool
				Temporary() bool
			}); ok && netErr.Timeout() && netErr.Temporary() {
				// UDP 超时错误，继续等待
				utils.Debugf("TunnelBridge[%s]: %s UDP timeout, continuing...", b.tunnelID, direction)
				continue
			}
			if err != io.EOF {
				utils.Debugf("TunnelBridge[%s]: %s read error: %v (total bytes: %d)", b.tunnelID, direction, err, total)
			}
			break
		}
	}

	return total
}

// reportTrafficStats 上报流量统计到CloudControl
func (b *TunnelBridge) reportTrafficStats() {
	if b.cloudControl == nil || b.mappingID == "" {
		return
	}

	// 获取当前累计值
	currentSent := b.bytesSent.Load()
	currentReceived := b.bytesReceived.Load()

	// 获取上次上报的值
	lastSent := b.lastReportedSent.Load()
	lastReceived := b.lastReportedReceived.Load()

	// 计算增量
	deltaSent := currentSent - lastSent
	deltaReceived := currentReceived - lastReceived

	// 如果没有增量，不上报
	if deltaSent == 0 && deltaReceived == 0 {
		return
	}

	// 获取当前映射的统计数据
	mapping, err := b.cloudControl.GetPortMapping(b.mappingID)
	if err != nil {
		utils.Errorf("TunnelBridge[%s]: failed to get mapping for traffic stats: %v", b.tunnelID, err)
		return
	}

	// 累加增量到映射统计
	trafficStats := mapping.TrafficStats
	trafficStats.BytesSent += deltaSent
	trafficStats.BytesReceived += deltaReceived
	trafficStats.LastUpdated = time.Now()

	// 更新映射统计
	if err := b.cloudControl.UpdatePortMappingStats(b.mappingID, &trafficStats); err != nil {
		utils.Errorf("TunnelBridge[%s]: failed to update traffic stats: %v", b.tunnelID, err)
		return
	}

	// 更新上次上报的值
	b.lastReportedSent.Store(currentSent)
	b.lastReportedReceived.Store(currentReceived)

	utils.Infof("TunnelBridge[%s]: traffic stats updated - mapping=%s, delta_sent=%d, delta_received=%d, total_sent=%d, total_received=%d",
		b.tunnelID, b.mappingID, deltaSent, deltaReceived, trafficStats.BytesSent, trafficStats.BytesReceived)
}

// Close 关闭桥接
func (b *TunnelBridge) Close() error {
	b.ManagerBase.Close()
	return nil
}

// dynamicSourceWriter 动态获取 sourceForwarder 的 Writer 包装器（使用接口抽象）
// 用于在 target->source 方向时，每次写入都使用最新的 sourceForwarder
type dynamicSourceWriter struct {
	bridge *TunnelBridge
}

func (w *dynamicSourceWriter) Write(p []byte) (n int, err error) {
	w.bridge.sourceConnMu.RLock()
	sourceForwarder := w.bridge.sourceForwarder
	w.bridge.sourceConnMu.RUnlock()

	if sourceForwarder == nil {
		return 0, io.ErrClosedPipe
	}
	return sourceForwarder.Write(p)
}
