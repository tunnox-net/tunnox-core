package bridge

import "time"

// MultiplexedConn 多路复用连接接口
type MultiplexedConn interface {
	RegisterSession(streamID string, session *ForwardSession) error
	UnregisterSession(streamID string)
	CanAcceptStream() bool
	GetActiveStreams() int32
	IsIdle(maxIdleTime time.Duration) bool
	GetTargetNodeID() string
	Close() error
	IsClosed() bool
}
