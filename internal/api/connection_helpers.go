package api

import (
	"fmt"
	"time"
	
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// ControlConnectionAccessor 控制连接访问器接口
type ControlConnectionAccessor interface {
	GetConnID() string
	GetRemoteAddr() string
	GetStream() *stream.StreamProcessor
}

// getStreamFromConnection 从控制连接获取StreamProcessor
// 返回streamProcessor, connID, remoteAddr, error
func getStreamFromConnection(connInterface interface{}, clientID int64) (*stream.StreamProcessor, string, string, error) {
	if connInterface == nil {
		return nil, "", "", fmt.Errorf("connection interface is nil")
	}

	// 尝试直接类型转换为ControlConnectionAccessor
	if accessor, ok := connInterface.(ControlConnectionAccessor); ok {
		stream := accessor.GetStream()
		if stream == nil {
			return nil, "", "", fmt.Errorf("stream is nil")
		}
		return stream, accessor.GetConnID(), accessor.GetRemoteAddr(), nil
	}

	// 回退方案：使用反射式接口
	var connID, remoteAddr string
	var streamProcessor *stream.StreamProcessor

	// 获取ConnID
	type hasConnID interface {
		GetConnID() string
	}
	if v, ok := connInterface.(hasConnID); ok {
		connID = v.GetConnID()
	}

	// 获取RemoteAddr
	type hasRemoteAddr interface {
		GetRemoteAddr() string
	}
	if v, ok := connInterface.(hasRemoteAddr); ok {
		remoteAddr = v.GetRemoteAddr()
	}

	// 获取Stream
	type hasGetStream interface {
		GetStream() interface{}
	}
	if hs, ok := connInterface.(hasGetStream); ok {
		streamInterface := hs.GetStream()
		if streamInterface == nil {
			return nil, connID, remoteAddr, fmt.Errorf("stream is nil")
		}

		var ok bool
		streamProcessor, ok = streamInterface.(*stream.StreamProcessor)
		if !ok {
			return nil, connID, remoteAddr, fmt.Errorf("stream type assertion failed, got type %T", streamInterface)
		}
	} else {
		return nil, connID, remoteAddr, fmt.Errorf("connection does not implement GetStream()")
	}

	return streamProcessor, connID, remoteAddr, nil
}

// sendPacketAsync 异步发送数据包（带超时）
func sendPacketAsync(streamProcessor *stream.StreamProcessor, pkt *packet.TransferPacket, clientID int64, timeout time.Duration) {
	go func() {
		done := make(chan error, 1)
		
		go func() {
			_, err := streamProcessor.WritePacket(pkt, false, 0)
			done <- err
		}()

		select {
		case err := <-done:
			if err != nil {
				utils.Errorf("API: failed to send packet to client %d: %v", clientID, err)
			} else {
				utils.Debugf("API: ✅ packet sent successfully to client %d", clientID)
			}
		case <-time.After(timeout):
			utils.Errorf("API: send packet to client %d timed out after %v", clientID, timeout)
		}
	}()
}

