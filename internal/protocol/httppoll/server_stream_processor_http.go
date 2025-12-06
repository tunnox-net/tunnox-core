package httppoll

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"time"

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
		utils.Infof("ServerStreamProcessor[%s]: HandlePushRequest - received push data, len=%d, connID=%s", sp.connectionID, len(pushReq.Data), sp.connectionID)
		// 尝试解析为分片数据（FragmentResponse JSON）
		fragmentResp, err := UnmarshalFragmentResponse([]byte(pushReq.Data))
		if err == nil && fragmentResp != nil && fragmentResp.TotalFragments > 1 {
			// 这是分片数据（TotalFragments > 1），使用 FragmentReassembler 重组
			utils.Infof("ServerStreamProcessor[%s]: HandlePushRequest - received fragment, groupID=%s, index=%d/%d, size=%d, originalSize=%d, connID=%s",
				sp.connectionID, fragmentResp.FragmentGroupID, fragmentResp.FragmentIndex, fragmentResp.TotalFragments, fragmentResp.FragmentSize, fragmentResp.OriginalSize, sp.connectionID)
			// 这是分片数据，使用 FragmentReassembler 重组
			// 解码分片数据
			fragmentData, err := base64.StdEncoding.DecodeString(fragmentResp.Data)
			if err != nil {
				return nil, fmt.Errorf("failed to decode fragment data: %w", err)
			}

			// 验证解码后的数据长度是否与 FragmentSize 匹配
			if len(fragmentData) != fragmentResp.FragmentSize {
				utils.Errorf("ServerStreamProcessor[%s]: HandlePushRequest - fragment size mismatch: expected %d, got %d, groupID=%s, index=%d",
					sp.connectionID, fragmentResp.FragmentSize, len(fragmentData), fragmentResp.FragmentGroupID, fragmentResp.FragmentIndex)
				return nil, fmt.Errorf("fragment size mismatch: expected %d, got %d", fragmentResp.FragmentSize, len(fragmentData))
			}

			// 添加到重组器
			// 注意：使用 FragmentSize 字段（这是实际数据长度，CreateFragmentResponse 中设置的）
			utils.Infof("ServerStreamProcessor[%s]: HandlePushRequest - adding fragment to reassembler, groupID=%s, index=%d/%d, size=%d, originalSize=%d, sequenceNumber=%d, connID=%s",
				sp.connectionID, fragmentResp.FragmentGroupID, fragmentResp.FragmentIndex, fragmentResp.TotalFragments, fragmentResp.FragmentSize, fragmentResp.OriginalSize, fragmentResp.SequenceNumber, sp.connectionID)
			group, err := sp.fragmentReassembler.AddFragment(
				fragmentResp.FragmentGroupID,
				fragmentResp.OriginalSize,
				fragmentResp.FragmentSize,
				fragmentResp.FragmentIndex,
				fragmentResp.TotalFragments,
				fragmentResp.SequenceNumber,
				fragmentData,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to add fragment: %w", err)
			}

			// 检查是否完整，如果完整则重组并推送
			if group.IsComplete() {
				utils.Infof("ServerStreamProcessor[%s]: HandlePushRequest - fragment group complete, reassembling, groupID=%s, receivedCount=%d/%d, connID=%s",
					sp.connectionID, fragmentResp.FragmentGroupID, group.ReceivedCount, fragmentResp.TotalFragments, sp.connectionID)
				reassembledData, err := group.Reassemble()
				if err != nil {
					utils.Errorf("ServerStreamProcessor[%s]: HandlePushRequest - failed to reassemble fragments: %v, groupID=%s, connID=%s",
						sp.connectionID, err, fragmentResp.FragmentGroupID, sp.connectionID)
					return nil, fmt.Errorf("failed to reassemble fragments: %w", err)
				}

				// 验证重组后的数据大小
				if len(reassembledData) != fragmentResp.OriginalSize {
					utils.Errorf("ServerStreamProcessor[%s]: HandlePushRequest - reassembled size mismatch: expected %d, got %d, groupID=%s, connID=%s",
						sp.connectionID, fragmentResp.OriginalSize, len(reassembledData), fragmentResp.FragmentGroupID, sp.connectionID)
					sp.fragmentReassembler.RemoveGroup(fragmentResp.FragmentGroupID)
					return nil, fmt.Errorf("reassembled size mismatch: expected %d, got %d", fragmentResp.OriginalSize, len(reassembledData))
				}

				// 将重组后的数据 Base64 编码并推送
				base64Data := base64.StdEncoding.EncodeToString(reassembledData)
				utils.Infof("ServerStreamProcessor[%s]: HandlePushRequest - pushing reassembled data, size=%d, base64Len=%d, groupID=%s, connID=%s",
					sp.connectionID, len(reassembledData), len(base64Data), fragmentResp.FragmentGroupID, sp.connectionID)
				if err := sp.PushData(base64Data); err != nil {
					utils.Errorf("ServerStreamProcessor[%s]: HandlePushRequest - failed to push reassembled data: %v, groupID=%s, connID=%s",
						sp.connectionID, err, fragmentResp.FragmentGroupID, sp.connectionID)
					return nil, fmt.Errorf("failed to push reassembled data: %w", err)
				}

				// 清理分片组
				sp.fragmentReassembler.RemoveGroup(fragmentResp.FragmentGroupID)
				utils.Infof("ServerStreamProcessor[%s]: HandlePushRequest - reassembled and pushed %d bytes successfully, groupID=%s, originalSize=%d, connID=%s",
					sp.connectionID, len(reassembledData), fragmentResp.FragmentGroupID, fragmentResp.OriginalSize, sp.connectionID)
			} else {
				utils.Infof("ServerStreamProcessor[%s]: HandlePushRequest - fragment %d/%d received, waiting for more, groupID=%s, receivedCount=%d, connID=%s",
					sp.connectionID, fragmentResp.FragmentIndex, fragmentResp.TotalFragments, fragmentResp.FragmentGroupID, group.ReceivedCount, sp.connectionID)
			}
		} else {
			// 这是完整数据（Base64 字符串或单分片数据），直接推送
			// 注意：如果 UnmarshalFragmentResponse 成功但 TotalFragments == 1，这也是完整数据
			if fragmentResp != nil && fragmentResp.TotalFragments == 1 {
				// 单分片数据，直接解码并推送
				utils.Infof("ServerStreamProcessor[%s]: HandlePushRequest - received single fragment (complete data), size=%d, groupID=%s, connID=%s",
					sp.connectionID, fragmentResp.FragmentSize, fragmentResp.FragmentGroupID, sp.connectionID)
				fragmentData, err := base64.StdEncoding.DecodeString(fragmentResp.Data)
				if err != nil {
					utils.Errorf("ServerStreamProcessor[%s]: HandlePushRequest - failed to decode single fragment data: %v, connID=%s", sp.connectionID, err, sp.connectionID)
					return nil, fmt.Errorf("failed to decode single fragment data: %w", err)
				}
				base64Data := base64.StdEncoding.EncodeToString(fragmentData)
				if err := sp.PushData(base64Data); err != nil {
					utils.Errorf("ServerStreamProcessor[%s]: HandlePushRequest - failed to push single fragment data: %v, connID=%s", sp.connectionID, err, sp.connectionID)
					return nil, fmt.Errorf("failed to push single fragment data: %w", err)
				}
				utils.Infof("ServerStreamProcessor[%s]: HandlePushRequest - pushed single fragment data, size=%d, connID=%s",
					sp.connectionID, len(fragmentData), sp.connectionID)
			} else {
				// 这是完整数据（Base64 字符串），直接推送
				utils.Infof("ServerStreamProcessor[%s]: HandlePushRequest - received complete data (not fragment), len=%d, connID=%s",
					sp.connectionID, len(pushReq.Data), sp.connectionID)
		if err := sp.PushData(pushReq.Data); err != nil {
					utils.Errorf("ServerStreamProcessor[%s]: HandlePushRequest - failed to push data: %v, connID=%s", sp.connectionID, err, sp.connectionID)
			return nil, fmt.Errorf("failed to push data: %w", err)
				}
				utils.Infof("ServerStreamProcessor[%s]: HandlePushRequest - pushed complete data, len=%d, connID=%s",
					sp.connectionID, len(pushReq.Data), sp.connectionID)
			}
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
	// 注意：keepalive 请求和 data 类型的 Poll 请求都会接收数据流，但它们使用不同的分片重组器
	// 这可能导致同一个分片组的分片被不同的连接接收，无法正确重组
	// 解决方案：优先让 data 类型的 Poll 请求接收数据流，keepalive 请求只在没有 data 类型请求时接收
	if tunnelType == "keepalive" {
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - keepalive request, checking for data stream, requestID=%s, connID=%s", actualRequestID, sp.connectionID)
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


	// 尝试匹配待分配的控制包（从 pendingControlPackets）
	// 由于 Poll 请求已注册，tryMatchControlPacket 应该能够匹配到
	sp.tryMatchControlPacket()

	// 先检查响应通道（可能已经匹配到控制包）
	select {
	case responsePkg := <-responseChan:
		// 从等待队列收到控制包（已匹配 requestID）
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - control packet received immediately from waiting queue (type=%s), connID=%s, requestID=%s",
			responsePkg.Type, sp.connectionID, actualRequestID)
		return "", responsePkg, nil
	default:
		// 没有控制包，继续等待
	}

	// 从队列获取数据流（非阻塞检查）
	var fragmentJSONStr string
	if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
		fragmentJSONStr = string(fragmentJSON)
		utils.Debugf("ServerStreamProcessor: HandlePollRequest - data stream received, len=%d, connID=%s", len(fragmentJSONStr), sp.connectionID)
		return fragmentJSONStr, nil, nil
	}

	// 队列为空，阻塞等待（控制包或数据流）
	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case <-sp.Ctx().Done():
		return "", nil, sp.Ctx().Err()
	case responsePkg := <-responseChan:
		// 从等待队列收到控制包（已匹配 requestID）
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
				return "", nil, ctx.Err()
			case <-sp.Ctx().Done():
				return "", nil, sp.Ctx().Err()
			case responsePkg := <-responseChan:
				// 从等待队列收到控制包（已匹配 requestID）
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

