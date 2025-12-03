package session

import (
	"net"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// StreamProcessorAccessor 类型别名，用于在接口定义中使用
type StreamProcessorAccessor = stream.StreamProcessorAccessor

// CreateTunnelConnection 从现有连接创建统一接口的隧道连接
// 根据协议类型自动选择合适的实现
func CreateTunnelConnection(
	connID string,
	netConn net.Conn,
	stream stream.PackageStreamer,
	clientID int64,
	mappingID string,
	tunnelID string,
) TunnelConnectionInterface {
	// 从 stream 获取协议类型
	protocol := extractProtocol(stream, netConn)

	switch protocol {
	case "httppoll", "http-long-polling", "httplp":
		// HTTP 长轮询：使用 ConnectionID，没有 net.Conn
		return NewHTTPPollTunnelConnection(connID, clientID, mappingID, tunnelID, stream)
	default:
		// TCP/WebSocket/QUIC：使用 net.Conn
		return NewTCPTunnelConnection(connID, netConn, clientID, mappingID, tunnelID, stream)
	}
}

// extractProtocol 从 stream 或 net.Conn 提取协议类型
func extractProtocol(stream stream.PackageStreamer, netConn net.Conn) string {
	// 尝试从 stream 获取协议类型
	if stream != nil {
		reader := stream.GetReader()
		if reader != nil {
			// 尝试从 ServerHTTPLongPollingConn 获取
			if httppollConn, ok := reader.(interface {
				GetConnectionID() string
				GetClientID() int64
			}); ok {
				// 检查是否有 ConnectionID（HTTP 长轮询特有）
				if httppollConn.GetConnectionID() != "" {
					return "httppoll"
				}
			}
		}
	}

	// 尝试从 net.Conn 获取协议类型
	if netConn != nil {
		addr := netConn.RemoteAddr()
		if addr != nil {
			network := addr.Network()
			switch network {
			case "tcp", "tcp4", "tcp6":
				return "tcp"
			case "udp", "udp4", "udp6":
				return "udp"
			case "ws", "wss":
				return "websocket"
			case "quic":
				return "quic"
			case "httppoll":
				return "httppoll"
			}
		}
	}

	// 默认返回 TCP
	return "tcp"
}

// extractClientID 从 stream 或 net.Conn 提取 clientID
func extractClientID(stream stream.PackageStreamer, netConn net.Conn) int64 {
	if stream != nil {
		reader := stream.GetReader()
		if reader != nil {
			// 尝试从 ServerHTTPLongPollingConn 获取
			if clientIDConn, ok := reader.(interface {
				GetClientID() int64
			}); ok {
				clientID := clientIDConn.GetClientID()
				if clientID > 0 {
					return clientID
				}
			}
		}

		// 尝试从 stream 直接获取
		if streamWithClientID, ok := stream.(interface {
			GetClientID() int64
		}); ok {
			clientID := streamWithClientID.GetClientID()
			if clientID > 0 {
				return clientID
			}
		}

		// 尝试从适配器获取
		type streamProcessorGetter interface {
			GetStreamProcessor() StreamProcessorAccessor
		}
		if adapter, ok := stream.(streamProcessorGetter); ok {
			streamProc := adapter.GetStreamProcessor()
			if streamProc != nil {
				clientID := streamProc.GetClientID()
				if clientID > 0 {
					return clientID
				}
			}
		}
	}

	return 0
}

// CreateTunnelConnectionFromExisting 从现有连接创建统一接口（自动提取信息）
func CreateTunnelConnectionFromExisting(
	connID string,
	netConn net.Conn,
	stream stream.PackageStreamer,
	mappingID string,
	tunnelID string,
) TunnelConnectionInterface {
	clientID := extractClientID(stream, netConn)
	if clientID == 0 {
		utils.Warnf("CreateTunnelConnectionFromExisting: failed to extract clientID, connID=%s", connID)
	}
	return CreateTunnelConnection(connID, netConn, stream, clientID, mappingID, tunnelID)
}

