package api

import (
	"fmt"
	"time"
	
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// getStreamFromConnection 从控制连接获取Stream
// 返回stream, connID, remoteAddr, error
func getStreamFromConnection(accessor ControlConnectionAccessor, clientID int64) (stream.PackageStreamer, string, string, error) {
	if accessor == nil {
		return nil, "", "", fmt.Errorf("connection accessor is nil")
	}

	stream := accessor.GetStream()
	if stream == nil {
		return nil, "", "", fmt.Errorf("stream is nil")
	}
	return stream, accessor.GetConnID(), accessor.GetRemoteAddr(), nil
}

// sendPacketAsync 异步发送数据包（带超时）
func sendPacketAsync(streamProcessor stream.PackageStreamer, pkt *packet.TransferPacket, clientID int64, timeout time.Duration) {
	go func() {
		done := make(chan error, 1)
		
		go func() {
			_, err := streamProcessor.WritePacket(pkt, true, 0)
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

