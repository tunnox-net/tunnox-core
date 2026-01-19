package session

import (
	"io"
	"sync"
	"sync/atomic"

	corelog "tunnox-core/internal/core/log"
)

// CountingReadWriter 带流量统计的读写器包装
type CountingReadWriter struct {
	rw         io.ReadWriter
	readBytes  *atomic.Int64 // 读取字节计数器（上传方向）
	writeBytes *atomic.Int64 // 写入字节计数器（下载方向）
}

// NewCountingReadWriter 创建带流量统计的读写器
func NewCountingReadWriter(rw io.ReadWriter, readCounter, writeCounter *atomic.Int64) *CountingReadWriter {
	return &CountingReadWriter{
		rw:         rw,
		readBytes:  readCounter,
		writeBytes: writeCounter,
	}
}

func (c *CountingReadWriter) Read(p []byte) (n int, err error) {
	n, err = c.rw.Read(p)
	if n > 0 && c.readBytes != nil {
		c.readBytes.Add(int64(n))
	}
	return
}

func (c *CountingReadWriter) Write(p []byte) (n int, err error) {
	n, err = c.rw.Write(p)
	if n > 0 && c.writeBytes != nil {
		c.writeBytes.Add(int64(n))
	}
	return
}

// HalfCloser 支持半关闭的接口
// 用于支持 HTTP 请求-响应模式：发送完请求后仍需接收响应
type HalfCloser interface {
	CloseWrite() error
}

// BidirectionalForwardConfig 双向转发配置
type BidirectionalForwardConfig struct {
	TunnelID   string
	LogPrefix  string
	LocalConn  io.ReadWriter
	RemoteConn io.ReadWriteCloser

	// 流量统计计数器（可选）
	BytesSentCounter     *atomic.Int64 // 源端->目标端（上传）
	BytesReceivedCounter *atomic.Int64 // 目标端->源端（下载）

	// LocalConnCloser 用于关闭本地连接（可选，用于确保 TCP 四次挥手完成）
	// 如果 LocalConn 本身实现了 io.Closer，可以不设置此字段
	LocalConnCloser io.Closer
}

// runBidirectionalForward 执行双向数据转发
// 这是 cross_node_session.go 和 cross_node_listener.go 的公共逻辑
// 支持半关闭语义：当一个方向完成时，发送半关闭信号而不是关闭整个连接
func runBidirectionalForward(config *BidirectionalForwardConfig) {
	done := make(chan struct{}, 2)
	var closeOnce sync.Once
	var uploadDone, downloadDone int32 // 原子标记

	logPrefix := config.LogPrefix
	if logPrefix == "" {
		logPrefix = "BidirectionalForward"
	}

	// closeAll 在两个方向都完成后关闭所有连接
	closeAll := func() {
		closeOnce.Do(func() {
			// 关闭远程连接（完全关闭）
			if err := config.RemoteConn.Close(); err != nil {
				corelog.Debugf("%s[%s]: remote close error (non-critical): %v",
					logPrefix, config.TunnelID, err)
			}
			// 关闭本地连接（确保 TCP 四次挥手完成）
			if config.LocalConnCloser != nil {
				if err := config.LocalConnCloser.Close(); err != nil {
					corelog.Debugf("%s[%s]: local close error (non-critical): %v",
						logPrefix, config.TunnelID, err)
				}
			} else if closer, ok := config.LocalConn.(io.Closer); ok {
				if err := closer.Close(); err != nil {
					corelog.Debugf("%s[%s]: local close error (non-critical): %v",
						logPrefix, config.TunnelID, err)
				}
			}
		})
	}

	// 包装 localConn 以统计流量
	localConn := config.LocalConn
	if config.BytesSentCounter != nil || config.BytesReceivedCounter != nil {
		localConn = NewCountingReadWriter(
			config.LocalConn,
			config.BytesSentCounter,     // Read from local = upload (sent)
			config.BytesReceivedCounter, // Write to local = download (received)
		)
	}

	// 上传方向: localConn -> remoteConn
	go func() {
		defer func() {
			atomic.StoreInt32(&uploadDone, 1)
			// 上传完成后，发送半关闭信号（如果支持）
			// 这告诉对端"我发送完毕，但仍在等待你的数据"
			if halfCloser, ok := config.RemoteConn.(HalfCloser); ok {
				if err := halfCloser.CloseWrite(); err != nil {
					corelog.Debugf("%s[%s]: remote half-close error (non-critical): %v",
						logPrefix, config.TunnelID, err)
				} else {
					corelog.Debugf("%s[%s]: sent half-close signal (upload done, waiting for download)",
						logPrefix, config.TunnelID)
				}
			}
			// 如果下载方向也完成了，关闭所有连接
			if atomic.LoadInt32(&downloadDone) == 1 {
				closeAll()
			}
			done <- struct{}{}
		}()
		n, err := io.Copy(config.RemoteConn, localConn)
		corelog.Infof("%s[%s]: upload copy ended: copied=%d bytes, err=%v",
			logPrefix, config.TunnelID, n, err)
	}()

	// 下载方向: remoteConn -> localConn
	go func() {
		defer func() {
			atomic.StoreInt32(&downloadDone, 1)
			// 如果上传方向也完成了，关闭所有连接
			if atomic.LoadInt32(&uploadDone) == 1 {
				closeAll()
			}
			done <- struct{}{}
		}()
		// 调试：记录 localConn 的类型
		corelog.Debugf("%s[%s]: download starting, localConn type=%T, remoteConn type=%T",
			logPrefix, config.TunnelID, localConn, config.RemoteConn)
		n, err := io.Copy(localConn, config.RemoteConn)
		corelog.Infof("%s[%s]: download copy ended: copied=%d bytes, err=%v",
			logPrefix, config.TunnelID, n, err)
	}()

	<-done
	corelog.Infof("%s[%s]: first direction completed, waiting for second direction",
		logPrefix, config.TunnelID)
	<-done
	corelog.Infof("%s[%s]: both directions completed, closing all connections",
		logPrefix, config.TunnelID)

	// 确保在函数返回前关闭所有连接
	closeAll()
}
