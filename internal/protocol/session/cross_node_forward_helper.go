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

// BidirectionalForwardConfig 双向转发配置
type BidirectionalForwardConfig struct {
	TunnelID   string
	LogPrefix  string
	LocalConn  io.ReadWriter
	RemoteConn io.ReadWriteCloser

	// 流量统计计数器（可选）
	BytesSentCounter     *atomic.Int64 // 源端->目标端（上传）
	BytesReceivedCounter *atomic.Int64 // 目标端->源端（下载）
}

// runBidirectionalForward 执行双向数据转发
// 这是 cross_node_session.go 和 cross_node_listener.go 的公共逻辑
func runBidirectionalForward(config *BidirectionalForwardConfig) {
	done := make(chan struct{}, 2)
	var closeOnce sync.Once

	logPrefix := config.LogPrefix
	if logPrefix == "" {
		logPrefix = "BidirectionalForward"
	}

	closeRemote := func() {
		closeOnce.Do(func() {
			if err := config.RemoteConn.Close(); err != nil {
				corelog.Debugf("%s[%s]: remote close error (non-critical): %v",
					logPrefix, config.TunnelID, err)
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
			closeRemote()
			done <- struct{}{}
		}()
		if _, err := io.Copy(config.RemoteConn, localConn); err != nil {
			corelog.Debugf("%s[%s]: upload copy ended: %v",
				logPrefix, config.TunnelID, err)
		}
	}()

	// 下载方向: remoteConn -> localConn
	go func() {
		defer func() {
			closeRemote()
			done <- struct{}{}
		}()
		if _, err := io.Copy(localConn, config.RemoteConn); err != nil {
			corelog.Debugf("%s[%s]: download copy ended: %v",
				logPrefix, config.TunnelID, err)
		}
	}()

	<-done
	<-done
}
