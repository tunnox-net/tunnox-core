package httppoll

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	coreErrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/utils"

	"github.com/google/uuid"
)

// startPollLoopWithRecovery 启动带有恢复机制的 pollLoop
// 如果 pollLoop panic 或因错误退出，会自动重启
func (sp *StreamProcessor) startPollLoopWithRecovery(pollID int) {
	// 错开启动时间，避免同时冲击服务器
	if pollID > 0 {
		time.Sleep(time.Duration(pollID) * 50 * time.Millisecond)
	}

	consecutiveErrors := 0
	maxConsecutiveErrors := 10

	for {
		// 检查是否应该退出
		select {
		case <-sp.Ctx().Done():
			utils.Infof("HTTPStreamProcessor: pollLoop %d exiting due to context cancellation, clientID=%d",
				pollID, sp.clientID)
			return
		default:
		}

		// 使用 defer + recover 捕获 panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					utils.Errorf("HTTPStreamProcessor: pollLoop %d panic: %v, will restart after delay, clientID=%d",
						pollID, r, sp.clientID)
					consecutiveErrors++
				}
			}()

			utils.Infof("HTTPStreamProcessor: pollLoop %d started, clientID=%d", pollID, sp.clientID)

			// 运行 pollLoop
			err := sp.pollLoopWithErrorTracking(pollID, &consecutiveErrors, maxConsecutiveErrors)
			if err != nil {
				utils.Errorf("HTTPStreamProcessor: pollLoop %d exited with error: %v, clientID=%d",
					pollID, err, sp.clientID)
				consecutiveErrors++
			}
		}()

		// 如果连续错误过多，增加重启延迟
		if consecutiveErrors > 0 {
			// 指数退避：100ms * 2^consecutiveErrors，最大 5 秒
			backoff := time.Duration(consecutiveErrors*consecutiveErrors) * 100 * time.Millisecond
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}

			utils.Warnf("HTTPStreamProcessor: pollLoop %d restarting after %v (consecutive errors: %d/%d), clientID=%d",
				pollID, backoff, consecutiveErrors, maxConsecutiveErrors, sp.clientID)

			select {
			case <-sp.Ctx().Done():
				return
			case <-time.After(backoff):
				// 继续重启
			}
		}

		// 如果连续错误达到阈值，触发更长的等待
		if consecutiveErrors >= maxConsecutiveErrors {
			utils.Errorf("HTTPStreamProcessor: pollLoop %d reached max consecutive errors (%d), waiting 30s before retry, clientID=%d",
				pollID, maxConsecutiveErrors, sp.clientID)

			select {
			case <-sp.Ctx().Done():
				return
			case <-time.After(30 * time.Second):
				// 重置错误计数，给系统一个新的机会
				consecutiveErrors = 0
			}
		}
	}
}

// pollLoopWithErrorTracking 带错误追踪的 pollLoop
// 返回 error 表示需要重启
func (sp *StreamProcessor) pollLoopWithErrorTracking(pollID int, consecutiveErrors *int, maxConsecutiveErrors int) error {
	for {
		select {
		case <-sp.Ctx().Done():
			return nil // 正常退出
		case requestID, ok := <-sp.pollRequestChan:
			if !ok {
				return coreErrors.New(coreErrors.ErrorTypePermanent, "pollRequestChan closed")
			}

			// 发送 Poll 请求
			err := sp.sendPollRequestWithErrorHandling(requestID, consecutiveErrors)
			if err != nil {
				// 检查是否是 EOF 或严重错误
				if err == io.EOF {
					utils.Warnf("HTTPStreamProcessor: pollLoop %d received EOF (consecutive errors: %d/%d), clientID=%d",
						pollID, *consecutiveErrors, maxConsecutiveErrors, sp.clientID)

					// EOF 后等待一段时间再继续
					if *consecutiveErrors > 3 {
						backoff := time.Duration(*consecutiveErrors) * 500 * time.Millisecond
						if backoff > 5*time.Second {
							backoff = 5 * time.Second
						}

						select {
						case <-sp.Ctx().Done():
							return nil
						case <-time.After(backoff):
							// 继续
						}
					}
				}

				// 检查是否需要触发重启
				if *consecutiveErrors >= maxConsecutiveErrors {
					return coreErrors.Newf(coreErrors.ErrorTypeTemporary,
						"too many consecutive errors (%d)", *consecutiveErrors)
				}
			} else {
				// 成功，重置错误计数
				*consecutiveErrors = 0
			}
		}
	}
}

// sendPollRequestWithErrorHandling 发送 Poll 请求并处理错误
// 返回 error 以便调用者追踪连续错误
func (sp *StreamProcessor) sendPollRequestWithErrorHandling(requestID string, consecutiveErrors *int) error {
	sp.closeMu.RLock()
	closed := sp.closed
	sp.closeMu.RUnlock()

	if closed {
		return io.EOF
	}

	// 调用原有的 sendPollRequest（它已经有重试逻辑）
	// 这里我们只是包装它来返回错误
	err := sp.sendPollRequestReturningError(requestID)
	if err != nil {
		*consecutiveErrors++
		return err
	}

	// 成功，重置连续错误计数
	*consecutiveErrors = 0
	return nil
}

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

// sendPollRequestReturningError 发送 Poll 请求并返回错误
// 这是 sendPollRequest 的错误返回版本，用于错误追踪
func (sp *StreamProcessor) sendPollRequestReturningError(requestID string) error {
	sp.closeMu.RLock()
	closed := sp.closed
	connID := sp.connectionID
	sp.closeMu.RUnlock()

	if closed {
		utils.Debugf("HTTPStreamProcessor: sendPollRequestReturningError - connection closed, requestID=%s", requestID)
		return io.EOF
	}

	// 指数退避重试配置
	baseDelay := 100 * time.Millisecond
	maxDelay := 1600 * time.Millisecond
	maxRetries := 5 // 100ms, 200ms, 400ms, 800ms, 1600ms

	var lastErr error
	for retry := 0; retry < maxRetries; retry++ {
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
			utils.Errorf("HTTPStreamProcessor: sendPollRequestReturningError - failed to encode poll package: %v, requestID=%s", err, requestID)
			return err
		}

		// 发送 Poll 请求
		req, err := http.NewRequestWithContext(sp.Ctx(), "GET", sp.pollURL+"?timeout=30", nil)
		if err != nil {
			utils.Errorf("HTTPStreamProcessor: sendPollRequestReturningError - failed to create poll request: %v, requestID=%s", err, requestID)
			return err
		}

		req.Header.Set("X-Tunnel-Package", encoded)
		if sp.token != "" {
			req.Header.Set("Authorization", "Bearer "+sp.token)
		}

		resp, err := sp.httpClient.Do(req)
		if err != nil {
			utils.Errorf("HTTPStreamProcessor: sendPollRequestReturningError - Poll request failed: %v, requestID=%s", err, requestID)
			lastErr = err
			// 网络错误,不重试(可能是连接断开)
			return lastErr
		}

		// 检查是否是限流错误(HTTP 429或503)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			resp.Body.Close()
			lastErr = coreErrors.Newf(coreErrors.ErrorTypeRateLimited, "poll rate limited, status=%d", resp.StatusCode)

			// 计算退避延迟: baseDelay * 2^retry
			delay := baseDelay * time.Duration(1<<uint(retry))
			if delay > maxDelay {
				delay = maxDelay
			}

			if retry < maxRetries-1 {
				utils.Warnf("HTTPStreamProcessor: sendPollRequestReturningError - rate limited (retry %d/%d), waiting %v, requestID=%s",
					retry+1, maxRetries, delay, requestID)

				// 等待退避时间
				select {
				case <-sp.Ctx().Done():
					return sp.Ctx().Err()
				case <-time.After(delay):
					// 继续重试
					continue
				}
			} else {
				utils.Errorf("HTTPStreamProcessor: sendPollRequestReturningError - rate limited after %d retries, requestID=%s", maxRetries, requestID)
				return lastErr
			}
		}

		// 成功获取响应,处理并返回
		defer resp.Body.Close()

		// 处理控制包响应（X-Tunnel-Package 中）
		sp.handleControlPacketResponse(resp, requestID)

		// 处理数据流（如果有）- 支持分片数据
		var pollResp FragmentResponse
		if err := json.NewDecoder(resp.Body).Decode(&pollResp); err == nil && pollResp.Data != "" {
			if err := sp.handleFragmentData(pollResp, requestID); err != nil {
				utils.Errorf("HTTPStreamProcessor: sendPollRequestReturningError - failed to handle fragment data: %v, requestID=%s", err, requestID)
				return err
			}
		} else if pollResp.Timeout {
			utils.Debugf("HTTPStreamProcessor: sendPollRequestReturningError - poll request timeout, requestID=%s", requestID)
		}

		// 成功处理,返回
		return nil
	}

	// 所有重试都失败
	if lastErr != nil {
		utils.Errorf("HTTPStreamProcessor: sendPollRequestReturningError - all retries failed: %v, requestID=%s", lastErr, requestID)
	}
	return lastErr
}

// sendPollRequest 发送单个 Poll 请求并缓存响应（不返回错误的版本，向后兼容）
func (sp *StreamProcessor) sendPollRequest(requestID string) {
	_ = sp.sendPollRequestReturningError(requestID)
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
		utils.Errorf("HTTPStreamProcessor: handleControlPacketResponse - failed to decode tunnel package: %v, requestID=%s", err, requestID)
		return
	}

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
}
