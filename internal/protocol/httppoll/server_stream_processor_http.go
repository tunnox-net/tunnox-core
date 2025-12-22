package httppoll

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"time"
)

// HTTPPushRequest HTTP 推送请求结构
type HTTPPushRequest struct {
	Data      string `json:"data"`
	Seq       uint64 `json:"seq"`
	Timestamp int64  `json:"timestamp"`
}

// HandlePushRequest 处理 HTTP Push 请求
func (sp *ServerStreamProcessor) HandlePushRequest(pkg *TunnelPackage, pushReq *HTTPPushRequest) (*TunnelPackage, error) {
	if pkg.ClientID > 0 {
		sp.UpdateClientID(pkg.ClientID)
	}
	if pkg.MappingID != "" {
		sp.SetMappingID(pkg.MappingID)
	}

	var responsePkg *TunnelPackage
	isControlPacket := pkg.Type != ""
	if isControlPacket {
		pkt, err := TunnelPackageToTransferPacket(pkg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert tunnel package: %w", err)
		}
		responsePkg = nil
		_ = pkt
	}

	if !isControlPacket && pushReq != nil && pushReq.Data != "" {
		fragmentResp, err := UnmarshalFragmentResponse([]byte(pushReq.Data))
		if err == nil && fragmentResp != nil && fragmentResp.TotalFragments > 1 {
			fragmentData, err := base64.StdEncoding.DecodeString(fragmentResp.Data)
			if err != nil {
				return nil, fmt.Errorf("failed to decode fragment data: %w", err)
			}

			if len(fragmentData) != fragmentResp.FragmentSize {
				return nil, fmt.Errorf("fragment size mismatch: expected %d, got %d", fragmentResp.FragmentSize, len(fragmentData))
			}

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

			if group.IsComplete() {
				reassembledData, err := group.Reassemble()
				if err != nil {
					return nil, fmt.Errorf("failed to reassemble fragments: %w", err)
				}

				if len(reassembledData) != fragmentResp.OriginalSize {
					sp.fragmentReassembler.RemoveGroup(fragmentResp.FragmentGroupID)
					return nil, fmt.Errorf("reassembled size mismatch: expected %d, got %d", fragmentResp.OriginalSize, len(reassembledData))
				}

				base64Data := base64.StdEncoding.EncodeToString(reassembledData)
				if err := sp.PushData(base64Data); err != nil {
					return nil, fmt.Errorf("failed to push reassembled data: %w", err)
				}

				sp.fragmentReassembler.RemoveGroup(fragmentResp.FragmentGroupID)
			}
		} else {
			if fragmentResp != nil && fragmentResp.TotalFragments == 1 {
				fragmentData, err := base64.StdEncoding.DecodeString(fragmentResp.Data)
				if err != nil {
					return nil, fmt.Errorf("failed to decode single fragment data: %w", err)
				}
				base64Data := base64.StdEncoding.EncodeToString(fragmentData)
				if err := sp.PushData(base64Data); err != nil {
					return nil, fmt.Errorf("failed to push single fragment data: %w", err)
				}
			} else {
				if err := sp.PushData(pushReq.Data); err != nil {
					return nil, fmt.Errorf("failed to push data: %w", err)
				}
			}
		}
	}

	return responsePkg, nil
}

// HandlePollRequest 处理 HTTP Poll 请求
func (sp *ServerStreamProcessor) HandlePollRequest(ctx context.Context, requestID string, tunnelType string) (string, *TunnelPackage, error) {
	actualRequestID := requestID
	if actualRequestID == "" {
		actualRequestID = "legacy-" + fmt.Sprintf("%d", time.Now().UnixNano())
	}

	if tunnelType == "keepalive" {
		if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
			return string(fragmentJSON), nil, nil
		}
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-sp.Ctx().Done():
			return "", nil, sp.Ctx().Err()
		case <-sp.pollWaitChan:
			if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
				return string(fragmentJSON), nil, nil
			}
			select {
			case <-ctx.Done():
				return "", nil, ctx.Err()
			case <-sp.Ctx().Done():
				return "", nil, sp.Ctx().Err()
			case data, ok := <-sp.pollDataChan:
				if !ok {
					return "", nil, io.EOF
				}
				return string(data), nil, nil
			case <-time.After(28 * time.Second):
				return "", nil, context.DeadlineExceeded
			}
		case data, ok := <-sp.pollDataChan:
			if !ok {
				return "", nil, io.EOF
			}
			return string(data), nil, nil
		case <-time.After(28 * time.Second):
			return "", nil, context.DeadlineExceeded
		}
	}

	responseChan := make(chan *TunnelPackage, 1)

	sp.pendingPollMu.Lock()
	sp.pendingPollRequests[actualRequestID] = &pollRequestInfo{
		responseChan: responseChan,
		tunnelType:   tunnelType,
	}
	sp.pendingPollMu.Unlock()

	defer func() {
		sp.pendingPollMu.Lock()
		if info, exists := sp.pendingPollRequests[actualRequestID]; exists {
			delete(sp.pendingPollRequests, actualRequestID)
			close(info.responseChan)
		}
		sp.pendingPollMu.Unlock()
	}()

	sp.tryMatchControlPacket()

	select {
	case responsePkg := <-responseChan:
		return "", responsePkg, nil
	default:
	}

	if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
		return string(fragmentJSON), nil, nil
	}

	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case <-sp.Ctx().Done():
		return "", nil, sp.Ctx().Err()
	case responsePkg := <-responseChan:
		return "", responsePkg, nil
	case <-sp.pollWaitChan:
		if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
			return string(fragmentJSON), nil, nil
		}
		sp.tryMatchControlPacket()
		select {
		case responsePkg := <-responseChan:
			return "", responsePkg, nil
		case data, ok := <-sp.pollDataChan:
			if !ok {
				return "", nil, io.EOF
			}
			return string(data), nil, nil
		default:
		}
		for {
			select {
			case <-ctx.Done():
				return "", nil, ctx.Err()
			case <-sp.Ctx().Done():
				return "", nil, sp.Ctx().Err()
			case responsePkg := <-responseChan:
				return "", responsePkg, nil
			case <-sp.pollWaitChan:
				if fragmentJSON, ok := sp.pollDataQueue.Pop(); ok {
					return string(fragmentJSON), nil, nil
				}
				sp.tryMatchControlPacket()
			case data, ok := <-sp.pollDataChan:
				if !ok {
					return "", nil, io.EOF
				}
				return string(data), nil, nil
			case <-time.After(100 * time.Millisecond):
				sp.tryMatchControlPacket()
			}
		}
	case data, ok := <-sp.pollDataChan:
		if !ok {
			return "", nil, io.EOF
		}
		return string(data), nil, nil
	}
}
