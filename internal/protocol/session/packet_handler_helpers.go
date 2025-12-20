package session

import (
	"net"

	"tunnox-core/internal/core/types"
)

// ToNetConn 统一接口：将适配层连接转换为 net.Conn
type ToNetConn interface {
	ToNetConn() net.Conn
}

// extractNetConn 从types.Connection中提取底层的net.Conn
func (s *SessionManager) extractNetConn(conn *types.Connection) net.Conn {
	if conn.RawConn != nil {
		return conn.RawConn
	}

	if conn.Stream != nil {
		// 使用接口获取 Reader，而不是类型断言
		reader := conn.Stream.GetReader()

		// 优先使用统一接口
		if toNetConn, ok := reader.(ToNetConn); ok {
			return toNetConn.ToNetConn()
		}

		// 回退：直接实现 net.Conn
		if netConn, ok := reader.(net.Conn); ok {
			return netConn
		}
	}
	return nil
}
