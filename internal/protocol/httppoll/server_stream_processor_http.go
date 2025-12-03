package httppoll

import (
	"context"
	"fmt"
	"io"
	"time"

	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// HTTPPushRequest HTTP 推送请求结构（用于服务端）
type HTTPPushRequest struct {
	Data      string `json:"data"`
	Seq       uint64 `json:"seq"`
	Timestamp int64  `json:"timestamp"`
}

// HandlePushRequest 处理 HTTP Push 请求（从 handleHTTPPush 调用）
func (sp *ServerStreamProcessor) HandlePushRequest(pkg *TunnelPackage, pushReq *HTTPPushRequest) (*TunnelPackage, error) {
	// 更新连接信息
	if pkg.ClientID > 0 {
		sp.UpdateClientID(pkg.ClientID)
	}
	if pkg.MappingID != "" {
		sp.SetMappingID(pkg.MappingID)
	}

	// 处理控制包
	var responsePkg *TunnelPackage
	if pkg.Type != "" {
		// 转换为 TransferPacket
		pkt, err := TunnelPackageToTransferPacket(pkg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert tunnel package: %w", err)
		}

		// 这里应该通过 SessionManager 处理包
		// 暂时返回 nil，由上层处理
		responsePkg = nil
		_ = pkt // 避免未使用变量
	}

	// 处理数据流
	if pushReq != nil && pushReq.Data != "" {
		if err := sp.PushData(pushReq.Data); err != nil {
			return nil, fmt.Errorf("failed to push data: %w", err)
		}
	}

	return responsePkg, nil
}

// HandlePollRequest 处理 HTTP Poll 请求（从 handleHTTPPoll 调用）
// requestID 是客户端请求中的 RequestId，用于在响应中携带
// tunnelType 是请求的 TunnelType（"control" | "data" | "keepalive"），用于区分请求类型
func (sp *ServerStreamProcessor) HandlePollRequest(ctx context.Context, requestID string, tunnelType string) (string, *TunnelPackage, error) {
	// 如果 requestID 为空，生成一个临时 ID（用于兼容旧代码）
	actualRequestID := requestID
	if actualRequestID == "" {
		actualRequestID = "legacy-" + fmt.Sprintf("%d", time.Now().UnixNano())
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - generated legacy requestID=%s, connID=%s", actualRequestID, sp.connectionID)
	}

	// keepalive 请求不应该注册到等待队列，因为它们不应该接收控制包
	// 但是它们应该能够接收数据流（从 pollDataQueue）
	if tunnelType == "keepalive" {
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - keepalive request, not registering for control packets, but checking for data stream, requestID=%s, connID=%s", actualRequestID, sp.connectionID)
		// keepalive 请求只等待数据流，不等待控制包
		// 先检查队列中是否有数据（非阻塞）
		if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
			// 返回分片响应的JSON字符串
			utils.Debugf("ServerStreamProcessor: HandlePollRequest - keepalive request received fragment from queue, len=%d, connID=%s", len(fragmentJSON), sp.connectionID)
			return string(fragmentJSON), nil, nil
		}
		// 队列为空，等待数据流（带超时）
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-sp.Ctx().Done():
			return "", nil, sp.Ctx().Err()
		case <-sp.pollWaitChan:
			// 收到信号，立即检查队列
			if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
				utils.Debugf("ServerStreamProcessor: HandlePollRequest - keepalive request received fragment after wait, len=%d, connID=%s", len(fragmentJSON), sp.connectionID)
				return string(fragmentJSON), nil, nil
			}
			// 如果队列仍为空，继续等待 pollDataChan
			select {
			case <-ctx.Done():
				return "", nil, ctx.Err()
			case <-sp.Ctx().Done():
				return "", nil, sp.Ctx().Err()
			case data, ok := <-sp.pollDataChan:
				if !ok {
					return "", nil, io.EOF
				}
				// pollDataChan 中的数据已经是 JSON 字节数组（由 pollDataScheduler 从 pollDataQueue Pop 出来的）
				// 直接返回，不需要再次包装
				utils.Debugf("ServerStreamProcessor: HandlePollRequest - keepalive request received data from pollDataChan, len=%d, connID=%s", len(data), sp.connectionID)
				return string(data), nil, nil
			case <-time.After(28 * time.Second):
				return "", nil, context.DeadlineExceeded
			}
		case data, ok := <-sp.pollDataChan:
			if !ok {
				return "", nil, io.EOF
			}
			// pollDataChan 中的数据已经是 JSON 字节数组（由 pollDataScheduler 从 pollDataQueue Pop 出来的）
			// 直接返回，不需要再次包装
			utils.Debugf("ServerStreamProcessor: HandlePollRequest - keepalive request received data from pollDataChan (immediate), len=%d, connID=%s", len(data), sp.connectionID)
			return string(data), nil, nil
		case <-time.After(28 * time.Second):
			return "", nil, context.DeadlineExceeded
		}
	}

	// 创建响应通道
	responseChan := make(chan *TunnelPackage, 1)

	// 注册等待请求（只注册非 keepalive 请求）
	sp.pendingPollMu.Lock()
	sp.pendingPollRequests[actualRequestID] = &pollRequestInfo{
		responseChan: responseChan,
		tunnelType:   tunnelType,
	}
	sp.pendingPollMu.Unlock()

	// 清理函数：如果请求超时或取消，从等待队列中移除
	defer func() {
		sp.pendingPollMu.Lock()
		if info, exists := sp.pendingPollRequests[actualRequestID]; exists {
			delete(sp.pendingPollRequests, actualRequestID)
			close(info.responseChan)
		}
		sp.pendingPollMu.Unlock()
	}()

	// [CMD_TRACE] 服务端 Poll 请求开始
	pollStartTime := time.Now()
	utils.Infof("[CMD_TRACE] [SERVER] [POLL_START] ConnID=%s, RequestID=%s, TunnelType=%s, Time=%s",
		sp.connectionID, actualRequestID, tunnelType, pollStartTime.Format("15:04:05.000"))

	// 尝试匹配待分配的控制包（从 pendingControlPackets）
	// 由于 Poll 请求已注册，tryMatchControlPacket 应该能够匹配到
	sp.tryMatchControlPacket()

	// 先检查响应通道（可能已经匹配到控制包）
	select {
	case responsePkg := <-responseChan:
		// 从等待队列收到控制包（已匹配 requestID）
		pollDuration := time.Since(pollStartTime)
		var responseType string
		var responseCommandID string
		if responsePkg != nil {
			responseType = responsePkg.Type
			if responsePkg.Data != nil {
				if cmdPkg, ok := responsePkg.Data.(*packet.CommandPacket); ok {
					responseCommandID = cmdPkg.CommandId
				}
			}
		}
		utils.Infof("[CMD_TRACE] [SERVER] [POLL_MATCHED_IMMEDIATE] ConnID=%s, RequestID=%s, ResponseType=%s, CommandID=%s, PollDuration=%v, Time=%s",
			sp.connectionID, actualRequestID, responseType, responseCommandID, pollDuration, time.Now().Format("15:04:05.000"))
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - control packet received immediately from waiting queue (type=%s), connID=%s, requestID=%s",
			responsePkg.Type, sp.connectionID, actualRequestID)
		return "", responsePkg, nil
	default:
		// 没有控制包，继续等待
		utils.Infof("[CMD_TRACE] [SERVER] [POLL_WAIT] ConnID=%s, RequestID=%s, Reason=no_immediate_control_packet, Time=%s",
			sp.connectionID, actualRequestID, time.Now().Format("15:04:05.000"))
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - no control packet immediately, waiting, connID=%s, requestID=%s", sp.connectionID, actualRequestID)
	}

	// 从队列获取数据流（非阻塞检查）
	var fragmentJSONStr string
	if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
		fragmentJSONStr = string(fragmentJSON)
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received, len=%d, connID=%s", len(fragmentJSONStr), sp.connectionID)
		return fragmentJSONStr, nil, nil
	}

	// 队列为空，阻塞等待（控制包或数据流）
	waitStartTime := time.Now()
	utils.Infof("[CMD_TRACE] [SERVER] [POLL_WAIT_START] ConnID=%s, RequestID=%s, Time=%s",
		sp.connectionID, actualRequestID, waitStartTime.Format("15:04:05.000"))
	select {
	case <-ctx.Done():
		waitDuration := time.Since(waitStartTime)
		pollDuration := time.Since(pollStartTime)
		utils.Infof("[CMD_TRACE] [SERVER] [POLL_TIMEOUT] ConnID=%s, RequestID=%s, WaitDuration=%v, PollDuration=%v, Reason=context_done, Time=%s",
			sp.connectionID, actualRequestID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
		return "", nil, ctx.Err()
	case <-sp.Ctx().Done():
		waitDuration := time.Since(waitStartTime)
		pollDuration := time.Since(pollStartTime)
		utils.Infof("[CMD_TRACE] [SERVER] [POLL_TIMEOUT] ConnID=%s, RequestID=%s, WaitDuration=%v, PollDuration=%v, Reason=connection_closed, Time=%s",
			sp.connectionID, actualRequestID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
		return "", nil, sp.Ctx().Err()
	case responsePkg := <-responseChan:
		// 从等待队列收到控制包（已匹配 requestID）
		waitDuration := time.Since(waitStartTime)
		pollDuration := time.Since(pollStartTime)
		var responseType string
		var responseCommandID string
		if responsePkg != nil {
			responseType = responsePkg.Type
			if responsePkg.Data != nil {
				if cmdPkg, ok := responsePkg.Data.(*packet.CommandPacket); ok {
					responseCommandID = cmdPkg.CommandId
				}
			}
		}
		utils.Infof("[CMD_TRACE] [SERVER] [POLL_MATCHED] ConnID=%s, RequestID=%s, ResponseType=%s, CommandID=%s, WaitDuration=%v, PollDuration=%v, Time=%s",
			sp.connectionID, actualRequestID, responseType, responseCommandID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - control packet received from waiting queue (type=%s), connID=%s, requestID=%s",
			responsePkg.Type, sp.connectionID, actualRequestID)
		return "", responsePkg, nil
	// 注意：不再使用 controlPacketChan，所有控制包都通过 pendingControlPackets 和 tryMatchControlPacket 匹配
	case <-sp.pollWaitChan:
		// 收到通知，检查队列
		if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
			fragmentJSONStr = string(fragmentJSON)
			utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received after wait, len=%d, connID=%s", len(fragmentJSONStr), sp.connectionID)
			return fragmentJSONStr, nil, nil
		}
		// 如果队列仍为空，尝试匹配控制包（可能新的控制包已到达）
		sp.tryMatchControlPacket()
		// 检查响应通道（可能已匹配到控制包）
		select {
		case responsePkg := <-responseChan:
			// 从等待队列收到控制包（已匹配 requestID）
			waitDuration := time.Since(waitStartTime)
			pollDuration := time.Since(pollStartTime)
			var responseType string
			var responseCommandID string
			if responsePkg != nil {
				responseType = responsePkg.Type
				if responsePkg.Data != nil {
					if cmdPkg, ok := responsePkg.Data.(*packet.CommandPacket); ok {
						responseCommandID = cmdPkg.CommandId
					}
				}
			}
			utils.Infof("[CMD_TRACE] [SERVER] [POLL_MATCHED] ConnID=%s, RequestID=%s, ResponseType=%s, CommandID=%s, WaitDuration=%v, PollDuration=%v, Time=%s",
				sp.connectionID, actualRequestID, responseType, responseCommandID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
			utils.Infof("ServerStreamProcessor: HandlePollRequest - control packet received from waiting queue (type=%s), connID=%s, requestID=%s",
				responsePkg.Type, sp.connectionID, actualRequestID)
			return "", responsePkg, nil
		case data, ok := <-sp.pollDataChan:
			if !ok {
				return "", nil, io.EOF
			}
			// pollDataChan 中的数据已经是 JSON 字节数组
			fragmentJSONStr = string(data)
			utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received from chan, len=%d, connID=%s", len(fragmentJSONStr), sp.connectionID)
			return fragmentJSONStr, nil, nil
		default:
			// 继续等待（回到外层 select 循环）
		}
		// 继续等待（回到外层 select，通过循环）
		// 使用 for 循环重新进入等待
		for {
			select {
			case <-ctx.Done():
				waitDuration := time.Since(waitStartTime)
				pollDuration := time.Since(pollStartTime)
				utils.Infof("[CMD_TRACE] [SERVER] [POLL_TIMEOUT] ConnID=%s, RequestID=%s, WaitDuration=%v, PollDuration=%v, Reason=context_done, Time=%s",
					sp.connectionID, actualRequestID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
				return "", nil, ctx.Err()
			case <-sp.Ctx().Done():
				waitDuration := time.Since(waitStartTime)
				pollDuration := time.Since(pollStartTime)
				utils.Infof("[CMD_TRACE] [SERVER] [POLL_TIMEOUT] ConnID=%s, RequestID=%s, WaitDuration=%v, PollDuration=%v, Reason=connection_closed, Time=%s",
					sp.connectionID, actualRequestID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
				return "", nil, sp.Ctx().Err()
			case responsePkg := <-responseChan:
				// 从等待队列收到控制包（已匹配 requestID）
				waitDuration := time.Since(waitStartTime)
				pollDuration := time.Since(pollStartTime)
				var responseType string
				var responseCommandID string
				if responsePkg != nil {
					responseType = responsePkg.Type
					if responsePkg.Data != nil {
						if cmdPkg, ok := responsePkg.Data.(*packet.CommandPacket); ok {
							responseCommandID = cmdPkg.CommandId
						}
					}
				}
				utils.Infof("[CMD_TRACE] [SERVER] [POLL_MATCHED] ConnID=%s, RequestID=%s, ResponseType=%s, CommandID=%s, WaitDuration=%v, PollDuration=%v, Time=%s",
					sp.connectionID, actualRequestID, responseType, responseCommandID, waitDuration, pollDuration, time.Now().Format("15:04:05.000"))
				utils.Infof("ServerStreamProcessor: HandlePollRequest - control packet received from waiting queue (type=%s), connID=%s, requestID=%s",
					responsePkg.Type, sp.connectionID, actualRequestID)
				return "", responsePkg, nil
			case <-sp.pollWaitChan:
				// 收到通知，检查队列
				if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
					fragmentJSONStr = string(fragmentJSON)
					utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received after wait, len=%d, connID=%s", len(fragmentJSONStr), sp.connectionID)
					return fragmentJSONStr, nil, nil
				}
				// 尝试匹配控制包
				sp.tryMatchControlPacket()
			case data, ok := <-sp.pollDataChan:
				if !ok {
					return "", nil, io.EOF
				}
				// pollDataChan 中的数据已经是 JSON 字节数组
				fragmentJSONStr = string(data)
				utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received from chan, len=%d, connID=%s", len(fragmentJSONStr), sp.connectionID)
				return fragmentJSONStr, nil, nil
			case <-time.After(100 * time.Millisecond):
				// 定期尝试匹配控制包
				sp.tryMatchControlPacket()
			}
		}
	case data, ok := <-sp.pollDataChan:
		if !ok {
			return "", nil, io.EOF
		}
		// pollDataChan 中的数据已经是 JSON 字节数组
		fragmentJSONStr = string(data)
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received from chan, len=%d, connID=%s", len(fragmentJSONStr), sp.connectionID)
		return fragmentJSONStr, nil, nil
	}
}

