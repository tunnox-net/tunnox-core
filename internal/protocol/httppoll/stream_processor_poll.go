package httppoll

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// pollLoop 持续发送 Poll 请求并缓存响应
func (sp *StreamProcessor) pollLoop() {
	// 对于数据连接（mappingID 不为空），启动持续的 Poll 循环
	if sp.mappingID != "" {
		go sp.dataPollLoop()
	}

	for {
		select {
		case <-sp.Ctx().Done():
			return
		case requestID, ok := <-sp.pollRequestChan:
			if !ok {
				return
			}
			sp.sendPollRequest(requestID)
		}
	}
}

// dataPollLoop 数据连接的持续 Poll 循环
func (sp *StreamProcessor) dataPollLoop() {
	// 等待启动信号（隧道建立后才开始 Poll）
	select {
	case <-sp.Ctx().Done():
		return
	case <-sp.dataPollStartCh:
	}

	for {
		select {
		case <-sp.Ctx().Done():
			return
		default:
		}

		sp.closeMu.RLock()
		closed := sp.closed
		sp.closeMu.RUnlock()
		if closed {
			return
		}

		requestID := uuid.New().String()
		sp.sendDataPollRequest(requestID)
	}
}

// sendDataPollRequest 发送数据 Poll 请求
func (sp *StreamProcessor) sendDataPollRequest(requestID string) {
	sp.closeMu.RLock()
	closed := sp.closed
	connID := sp.connectionID
	sp.closeMu.RUnlock()

	if closed {
		return
	}

	pollPkg := &TunnelPackage{
		ConnectionID: connID,
		RequestID:    requestID,
		ClientID:     sp.clientID,
		MappingID:    sp.mappingID,
		TunnelType:   "data",
	}
	encoded, err := EncodeTunnelPackage(pollPkg)
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(sp.Ctx(), "GET", sp.pollURL+"?timeout=30", nil)
	if err != nil {
		return
	}

	req.Header.Set("X-Tunnel-Package", encoded)
	if sp.token != "" {
		req.Header.Set("Authorization", "Bearer "+sp.token)
	}

	resp, err := sp.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var pollResp FragmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&pollResp); err == nil && pollResp.Data != "" {
		sp.handleFragmentData(pollResp, requestID)
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

// StartDataPoll 启动数据 Poll 循环（隧道建立后调用）
func (sp *StreamProcessor) StartDataPoll() {
	sp.dataPollStartMu.Lock()
	defer sp.dataPollStartMu.Unlock()

	if sp.dataPollStarted {
		return
	}
	sp.dataPollStarted = true
	close(sp.dataPollStartCh)
}

// sendPollRequest 发送单个 Poll 请求并缓存响应
func (sp *StreamProcessor) sendPollRequest(requestID string) {
	sp.closeMu.RLock()
	closed := sp.closed
	connID := sp.connectionID
	sp.closeMu.RUnlock()

	if closed {
		return
	}

	pollPkg := &TunnelPackage{
		ConnectionID: connID,
		RequestID:    requestID,
		ClientID:     sp.clientID,
		MappingID:    sp.mappingID,
		TunnelType:   sp.tunnelType,
	}
	encoded, err := EncodeTunnelPackage(pollPkg)
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(sp.Ctx(), "GET", sp.pollURL+"?timeout=30", nil)
	if err != nil {
		return
	}

	req.Header.Set("X-Tunnel-Package", encoded)
	if sp.token != "" {
		req.Header.Set("Authorization", "Bearer "+sp.token)
	}

	resp, err := sp.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	sp.handleControlPacketResponse(resp, requestID)

	var pollResp FragmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&pollResp); err == nil && pollResp.Data != "" {
		sp.handleFragmentData(pollResp, requestID)
	}
}

// handleControlPacketResponse 处理控制包响应
func (sp *StreamProcessor) handleControlPacketResponse(resp *http.Response, requestID string) {
	xTunnelPackage := resp.Header.Get("X-Tunnel-Package")
	if xTunnelPackage == "" {
		return
	}

	pkg, err := DecodeTunnelPackage(xTunnelPackage)
	if err != nil {
		return
	}

	if pkg.RequestID != requestID {
		return
	}

	pkt, err := sp.converter.ReadPacket(resp)
	if err != nil {
		return
	}

	if pkg.ConnectionID != "" {
		sp.SetConnectionID(pkg.ConnectionID)
	}

	sp.setCachedResponse(requestID, pkt)
}
