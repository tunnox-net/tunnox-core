package httppoll

import (
corelog "tunnox-core/internal/core/log"
	"encoding/json"
	"strings"

	"tunnox-core/internal/packet"
)

// tryMatchControlPacket 尝试将待分配的控制包匹配给等待的 Poll 请求
// 每次调用处理所有可匹配的控制包，直到没有等待的 Poll 请求或没有待分配的控制包
func (sp *ServerStreamProcessor) tryMatchControlPacket() {
	for {
		sp.pendingControlMu.Lock()
		if len(sp.pendingControlPackets) == 0 {
			sp.pendingControlMu.Unlock()
			return
		}
		// 取出第一个控制包
		controlPkt := sp.pendingControlPackets[0]
		sp.pendingControlPackets = sp.pendingControlPackets[1:]
		pendingCount := len(sp.pendingControlPackets)
		sp.pendingControlMu.Unlock()

		responsePkg := sp.transferPacketToTunnelPackage(controlPkt)

		// 检查是否有等待的 Poll 请求（优先匹配有 requestID 的，且不是 keepalive 类型）
		sp.pendingPollMu.Lock()
		var targetChan chan *TunnelPackage
		var targetRequestID string
		var availablePollCount int
		var keepaliveCount int
		// 记录所有等待的 Poll 请求信息
		for reqID, info := range sp.pendingPollRequests {
			availablePollCount++
			if info.tunnelType == "keepalive" {
				keepaliveCount++
			}
			if reqID != "" && !strings.HasPrefix(reqID, "legacy-") && info.tunnelType != "keepalive" {
				targetChan = info.responseChan
				targetRequestID = reqID
				if responsePkg != nil {
					responsePkg.RequestID = reqID
				}
				delete(sp.pendingPollRequests, reqID)
				break
			}
		}

		// 如果没有有 requestID 的请求，选择第一个非 keepalive 的请求
		if targetChan == nil {
			for reqID, info := range sp.pendingPollRequests {
				if info.tunnelType != "keepalive" {
					targetChan = info.responseChan
					targetRequestID = reqID
					if responsePkg != nil && reqID != "" {
						responsePkg.RequestID = reqID
					}
					delete(sp.pendingPollRequests, reqID)
					break
				}
			}
		}
		sp.pendingPollMu.Unlock()

		if targetChan != nil {
			// 有等待的请求，直接发送（使用该请求的 requestID）
			select {
			case targetChan <- responsePkg:
				corelog.Debugf("ServerStreamProcessor: tryMatchControlPacket - control packet matched to waiting Poll request, requestID=%s, connID=%s, remainingPackets=%d",
					targetRequestID, sp.connectionID, pendingCount)
				// 继续循环，尝试匹配下一个控制包
				continue
			default:
				// 通道已关闭或满，重新放回队列
				sp.pendingControlMu.Lock()
				sp.pendingControlPackets = append([]*packet.TransferPacket{controlPkt}, sp.pendingControlPackets...)
				sp.pendingControlMu.Unlock()
				corelog.Warnf("ServerStreamProcessor: tryMatchControlPacket - response channel full, requeued, requestID=%s", targetRequestID)
				return // 通道满，停止匹配
			}
		} else {
			// 没有等待的请求，重新放回队列（而不是放入 controlPacketChan，避免 FIFO 匹配错误）
			sp.pendingControlMu.Lock()
			sp.pendingControlPackets = append([]*packet.TransferPacket{controlPkt}, sp.pendingControlPackets...)
			pendingCount = len(sp.pendingControlPackets)
			sp.pendingControlMu.Unlock()
			corelog.Debugf("ServerStreamProcessor: tryMatchControlPacket - control packet requeued (no waiting requests), connID=%s, remainingPackets=%d", sp.connectionID, pendingCount)
			return // 没有等待的请求，停止匹配
		}
	}
}

// transferPacketToTunnelPackage 将 TransferPacket 转换为 TunnelPackage
func (sp *ServerStreamProcessor) transferPacketToTunnelPackage(pkt *packet.TransferPacket) *TunnelPackage {
	baseType := byte(pkt.PacketType) & 0x3F

	responsePkg := &TunnelPackage{
		ConnectionID: sp.connectionID,
		ClientID:     sp.clientID,
		MappingID:    sp.mappingID,
		TunnelType:   sp.tunnelType, // 保持为 "control" 或 "data"，不是 "keepalive"
		// 注意：即使是通过 keepalive 请求返回的，响应包本身也是指令包，TunnelType 应该是 "control" 或 "data"
	}

	// 根据包类型设置 Type 和 Data
	if baseType == byte(packet.HandshakeResp) {
		responsePkg.Type = "HandshakeResponse"
		var handshakeResp packet.HandshakeResponse
		if err := json.Unmarshal(pkt.Payload, &handshakeResp); err == nil {
			responsePkg.Data = &handshakeResp
		}
	} else if baseType == byte(packet.TunnelOpenAck) {
		responsePkg.Type = "TunnelOpenAck"
		var tunnelOpenAck packet.TunnelOpenAckResponse
		if err := json.Unmarshal(pkt.Payload, &tunnelOpenAck); err == nil {
			responsePkg.Data = &tunnelOpenAck
		}
	} else if pkt.PacketType.IsCommandResp() {
		responsePkg.Type = "CommandResponse"
		if pkt.CommandPacket != nil {
			responsePkg.Data = pkt.CommandPacket
		}
	} else if pkt.PacketType.IsJsonCommand() {
		responsePkg.Type = "JsonCommand"
		if pkt.CommandPacket != nil {
			responsePkg.Data = pkt.CommandPacket
		}
	}

	return responsePkg
}

