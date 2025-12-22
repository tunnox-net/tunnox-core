package httppoll

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"tunnox-core/internal/packet"
)

// tryMatchControlPacket 尝试将待分配的控制包匹配给等待的 Poll 请求
func (sp *ServerStreamProcessor) tryMatchControlPacket() {
	for {
		sp.pendingControlMu.Lock()
		if len(sp.pendingControlPackets) == 0 {
			sp.pendingControlMu.Unlock()
			return
		}
		controlPkt := sp.pendingControlPackets[0]
		sp.pendingControlPackets = sp.pendingControlPackets[1:]
		sp.pendingControlMu.Unlock()

		responsePkg := sp.transferPacketToTunnelPackage(controlPkt)

		sp.pendingPollMu.Lock()
		var targetChan chan *TunnelPackage

		for reqID, info := range sp.pendingPollRequests {
			if info.tunnelType == "control" {
				if reqID != "" && !strings.HasPrefix(reqID, "legacy-") {
					targetChan = info.responseChan
					if responsePkg != nil {
						responsePkg.RequestID = reqID
					}
					delete(sp.pendingPollRequests, reqID)
					break
				}
			}
		}

		if targetChan == nil {
			for reqID, info := range sp.pendingPollRequests {
				if info.tunnelType == "control" {
					targetChan = info.responseChan
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
			select {
			case targetChan <- responsePkg:
				continue
			default:
				sp.pendingControlMu.Lock()
				sp.pendingControlPackets = append([]*packet.TransferPacket{controlPkt}, sp.pendingControlPackets...)
				sp.pendingControlMu.Unlock()
				return
			}
		} else {
			sp.pendingControlMu.Lock()
			sp.pendingControlPackets = append([]*packet.TransferPacket{controlPkt}, sp.pendingControlPackets...)
			sp.pendingControlMu.Unlock()
			return
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
		TunnelType:   sp.tunnelType,
	}

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

// WaitForControlPacket 等待并获取控制包响应
func (sp *ServerStreamProcessor) WaitForControlPacket(ctx context.Context, timeout time.Duration) *TunnelPackage {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return nil
		case <-ticker.C:
			sp.pendingControlMu.Lock()
			if len(sp.pendingControlPackets) > 0 {
				controlPkt := sp.pendingControlPackets[0]
				sp.pendingControlPackets = sp.pendingControlPackets[1:]
				sp.pendingControlMu.Unlock()
				return sp.transferPacketToTunnelPackage(controlPkt)
			}
			sp.pendingControlMu.Unlock()
		case <-sp.pollWaitChan:
			sp.pendingControlMu.Lock()
			if len(sp.pendingControlPackets) > 0 {
				controlPkt := sp.pendingControlPackets[0]
				sp.pendingControlPackets = sp.pendingControlPackets[1:]
				sp.pendingControlMu.Unlock()
				return sp.transferPacketToTunnelPackage(controlPkt)
			}
			sp.pendingControlMu.Unlock()
		}
	}
}
