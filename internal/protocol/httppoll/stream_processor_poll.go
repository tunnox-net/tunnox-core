package httppoll

import (
	"encoding/json"
	"net/http"
	"time"

	"tunnox-core/internal/utils"

	"github.com/google/uuid"
)

// pollLoop 持续发送 Poll 请求并缓存响应
func (sp *StreamProcessor) pollLoop() {
	for {
		select {
		case <-sp.Ctx().Done():
			return
		case requestID, ok := <-sp.pollRequestChan:
			if !ok {
				return
			}
			// 发送 Poll 请求
			sp.sendPollRequest(requestID)
		}
	}
}

// TriggerImmediatePoll 立即触发一个 Poll 请求（用于发送命令后快速获取响应）
// 返回的 RequestID 应该被 ReadPacket 使用
func (sp *StreamProcessor) TriggerImmediatePoll() string {
	requestID := uuid.New().String()
	// 设置待使用的 RequestID
	sp.pendingPollRequestMu.Lock()
	sp.pendingPollRequestID = requestID
	sp.pendingPollRequestMu.Unlock()

	select {
	case sp.pollRequestChan <- requestID:
		utils.Infof("[CMD_TRACE] [CLIENT] [TRIGGER_POLL_IMMEDIATE] RequestID=%s, ConnID=%s, Time=%s",
			requestID, sp.connectionID, time.Now().Format("15:04:05.000"))
		return requestID
	case <-sp.Ctx().Done():
		sp.pendingPollRequestMu.Lock()
		sp.pendingPollRequestID = ""
		sp.pendingPollRequestMu.Unlock()
		return ""
	default:
		// 通道满，清除待使用的 RequestID
		sp.pendingPollRequestMu.Lock()
		sp.pendingPollRequestID = ""
		sp.pendingPollRequestMu.Unlock()
		utils.Warnf("[CMD_TRACE] [CLIENT] [TRIGGER_POLL_IMMEDIATE_WARN] RequestID=%s, Reason=pollRequestChan_full, Time=%s",
			requestID, time.Now().Format("15:04:05.000"))
		return ""
	}
}

// sendPollRequest 发送单个 Poll 请求并缓存响应
func (sp *StreamProcessor) sendPollRequest(requestID string) {
	sp.closeMu.RLock()
	closed := sp.closed
	connID := sp.connectionID
	sp.closeMu.RUnlock()

	if closed {
		utils.Debugf("HTTPStreamProcessor: sendPollRequest - connection closed, requestID=%s", requestID)
		return
	}

	pollStartTime := time.Now()
	utils.Infof("[CMD_TRACE] [CLIENT] [POLL_START] RequestID=%s, ConnID=%s, Time=%s",
		requestID, connID, pollStartTime.Format("15:04:05.000"))

	// 构建 Poll 请求的 TunnelPackage
	pollPkg := &TunnelPackage{
		ConnectionID: connID,
		RequestID:    requestID,
		ClientID:     sp.clientID,
		MappingID:    sp.mappingID,
		TunnelType:   sp.tunnelType,
	}
	encoded, err := EncodeTunnelPackage(pollPkg)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to encode poll package: %v, requestID=%s", err, requestID)
		return
	}

	// 发送 Poll 请求
	req, err := http.NewRequestWithContext(sp.Ctx(), "GET", sp.pollURL+"?timeout=30", nil)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to create poll request: %v, requestID=%s", err, requestID)
		return
	}

	req.Header.Set("X-Tunnel-Package", encoded)
	if sp.token != "" {
		req.Header.Set("Authorization", "Bearer "+sp.token)
	}

	utils.Infof("HTTPStreamProcessor: sendPollRequest - Poll request sent, requestID=%s, encodedLen=%d", requestID, len(encoded))
	resp, err := sp.httpClient.Do(req)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor: sendPollRequest - Poll request failed: %v, requestID=%s", err, requestID)
		return
	}
	defer resp.Body.Close()

	utils.Infof("HTTPStreamProcessor: sendPollRequest - Poll response received, status=%d, requestID=%s", resp.StatusCode, requestID)

	// 处理控制包响应（X-Tunnel-Package 中）
	sp.handleControlPacketResponse(resp, requestID)

	// 处理数据流（如果有）- 支持分片数据
	var pollResp FragmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&pollResp); err == nil && pollResp.Data != "" {
		if err := sp.handleFragmentData(pollResp, requestID); err != nil {
			utils.Errorf("HTTPStreamProcessor: sendPollRequest - failed to handle fragment data: %v, requestID=%s", err, requestID)
		}
	} else if pollResp.Timeout {
		utils.Debugf("HTTPStreamProcessor: sendPollRequest - poll request timeout, requestID=%s", requestID)
	}
}

// handleControlPacketResponse 处理控制包响应
func (sp *StreamProcessor) handleControlPacketResponse(resp *http.Response, requestID string) {
	// 检查是否有控制包（X-Tunnel-Package 中）
	xTunnelPackage := resp.Header.Get("X-Tunnel-Package")
	utils.Infof("HTTPStreamProcessor: handleControlPacketResponse - checking X-Tunnel-Package header, present=%v, len=%d, requestID=%s",
		xTunnelPackage != "", len(xTunnelPackage), requestID)
	if xTunnelPackage == "" {
		return
	}

	// 解码 TunnelPackage 以检查 RequestId
	pkg, err := DecodeTunnelPackage(xTunnelPackage)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor: handleControlPacketResponse - failed to decode tunnel package: %v, requestID=%s", err, requestID)
		return
	}

	utils.Infof("HTTPStreamProcessor: handleControlPacketResponse - decoded tunnel package, requestID in response=%s, expected=%s",
		pkg.RequestID, requestID)

	// 检查 RequestId 是否匹配
	if pkg.RequestID != requestID {
		utils.Warnf("HTTPStreamProcessor: handleControlPacketResponse - RequestId mismatch, expected=%s, got=%s, ignoring response",
			requestID, pkg.RequestID)
		return
	}

	// 转换为 TransferPacket
	pkt, err := sp.converter.ReadPacket(resp)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor: handleControlPacketResponse - failed to read packet: %v, requestID=%s", err, requestID)
		return
	}

	// 更新连接信息
	if pkg.ConnectionID != "" {
		sp.SetConnectionID(pkg.ConnectionID)
	}

	// 缓存响应
	sp.setCachedResponse(requestID, pkt)

	utils.Infof("HTTPStreamProcessor: handleControlPacketResponse - cached response, requestID=%s, type=0x%02x",
		requestID, byte(pkt.PacketType)&0x3F)
}

