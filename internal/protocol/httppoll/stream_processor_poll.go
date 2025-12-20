package httppoll

import (
corelog "tunnox-core/internal/core/log"
	"encoding/json"
	"net/http"


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
		corelog.Debugf("HTTPStreamProcessor: sendPollRequest - connection closed, requestID=%s", requestID)
		return
	}

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
		corelog.Errorf("HTTPStreamProcessor: sendPollRequest - failed to encode poll package: %v, requestID=%s", err, requestID)
		return
	}

	// 发送 Poll 请求
	req, err := http.NewRequestWithContext(sp.Ctx(), "GET", sp.pollURL+"?timeout=30", nil)
	if err != nil {
		corelog.Errorf("HTTPStreamProcessor: sendPollRequest - failed to create poll request: %v, requestID=%s", err, requestID)
		return
	}

	req.Header.Set("X-Tunnel-Package", encoded)
	if sp.token != "" {
		req.Header.Set("Authorization", "Bearer "+sp.token)
	}

	resp, err := sp.httpClient.Do(req)
	if err != nil {
		corelog.Errorf("HTTPStreamProcessor: sendPollRequest - Poll request failed: %v, requestID=%s", err, requestID)
		return
	}
	defer resp.Body.Close()

	// 处理控制包响应（X-Tunnel-Package 中）
	sp.handleControlPacketResponse(resp, requestID)

	// 处理数据流（如果有）- 支持分片数据
	var pollResp FragmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&pollResp); err == nil && pollResp.Data != "" {
		if err := sp.handleFragmentData(pollResp, requestID); err != nil {
			corelog.Errorf("HTTPStreamProcessor: sendPollRequest - failed to handle fragment data: %v, requestID=%s", err, requestID)
		}
	} else if pollResp.Timeout {
		corelog.Debugf("HTTPStreamProcessor: sendPollRequest - poll request timeout, requestID=%s", requestID)
	}
}

// handleControlPacketResponse 处理控制包响应
func (sp *StreamProcessor) handleControlPacketResponse(resp *http.Response, requestID string) {
	// 检查是否有控制包（X-Tunnel-Package 中）
	xTunnelPackage := resp.Header.Get("X-Tunnel-Package")
	if xTunnelPackage == "" {
		return
	}

	// 解码 TunnelPackage 以检查 RequestId
	pkg, err := DecodeTunnelPackage(xTunnelPackage)
	if err != nil {
		corelog.Errorf("HTTPStreamProcessor: handleControlPacketResponse - failed to decode tunnel package: %v, requestID=%s", err, requestID)
		return
	}

	// 检查 RequestId 是否匹配
	if pkg.RequestID != requestID {
		corelog.Warnf("HTTPStreamProcessor: handleControlPacketResponse - RequestId mismatch, expected=%s, got=%s, ignoring response",
			requestID, pkg.RequestID)
		return
	}

	// 转换为 TransferPacket
	pkt, err := sp.converter.ReadPacket(resp)
	if err != nil {
		corelog.Errorf("HTTPStreamProcessor: handleControlPacketResponse - failed to read packet: %v, requestID=%s", err, requestID)
		return
	}

	// 更新连接信息
	if pkg.ConnectionID != "" {
		sp.SetConnectionID(pkg.ConnectionID)
	}

	// 缓存响应
	sp.setCachedResponse(requestID, pkt)
}
