package connection

import (
	"net"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream"
)

// ============================================================================
// 连接工厂
// ============================================================================

// StreamProcessorAccessor 类型别名
type StreamProcessorAccessor = stream.StreamProcessorAccessor

// CreateTunnelConnection 从现有连接创建统一接口的隧道连接
func CreateTunnelConnection(
	connID string,
	netConn net.Conn,
	stream stream.PackageStreamer,
	clientID int64,
	mappingID string,
	tunnelID string,
) TunnelConnectionInterface {
	return NewTCPTunnelConnection(connID, netConn, clientID, mappingID, tunnelID, stream)
}

// extractClientID 从 stream 或 net.Conn 提取 clientID
func extractClientID(stream stream.PackageStreamer, netConn net.Conn) int64 {
	if stream != nil {
		reader := stream.GetReader()
		if reader != nil {
			if clientIDConn, ok := reader.(interface {
				GetClientID() int64
			}); ok {
				clientID := clientIDConn.GetClientID()
				if clientID > 0 {
					return clientID
				}
			}
		}

		if streamWithClientID, ok := stream.(interface {
			GetClientID() int64
		}); ok {
			clientID := streamWithClientID.GetClientID()
			if clientID > 0 {
				return clientID
			}
		}

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
		corelog.Warnf("CreateTunnelConnectionFromExisting: failed to extract clientID, connID=%s", connID)
	}
	return CreateTunnelConnection(connID, netConn, stream, clientID, mappingID, tunnelID)
}
