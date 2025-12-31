package session

import (
	"io"
	"sync"

	corelog "tunnox-core/internal/core/log"
)

// BidirectionalForwardConfig 双向转发配置
type BidirectionalForwardConfig struct {
	TunnelID   string
	LogPrefix  string
	LocalConn  io.ReadWriter
	RemoteConn io.ReadWriteCloser
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

	// 上传方向: localConn -> remoteConn
	go func() {
		defer func() {
			closeRemote()
			done <- struct{}{}
		}()
		if _, err := io.Copy(config.RemoteConn, config.LocalConn); err != nil {
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
		if _, err := io.Copy(config.LocalConn, config.RemoteConn); err != nil {
			corelog.Debugf("%s[%s]: download copy ended: %v",
				logPrefix, config.TunnelID, err)
		}
	}()

	<-done
	<-done
}
