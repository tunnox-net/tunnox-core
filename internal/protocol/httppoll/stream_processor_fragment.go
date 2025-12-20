package httppoll

import (
	"encoding/base64"
	"fmt"
	corelog "tunnox-core/internal/core/log"
)

// handleFragmentData 处理分片数据（从sendPollRequest中提取）
func (sp *StreamProcessor) handleFragmentData(pollResp FragmentResponse, requestID string) error {
	// 判断是否为分片：total_fragments > 1
	isFragment := pollResp.TotalFragments > 1

	// 解码Base64数据
	fragmentData, err := base64.StdEncoding.DecodeString(pollResp.Data)
	if err != nil {
		corelog.Errorf("HTTPStreamProcessor: handleFragmentData - failed to decode fragment data: %v, requestID=%s", err, requestID)
		return fmt.Errorf("failed to decode fragment data: %w", err)
	}

	// 验证解码后的数据长度是否与 FragmentSize 匹配
	if len(fragmentData) != pollResp.FragmentSize {
		corelog.Errorf("HTTPStreamProcessor: handleFragmentData - fragment size mismatch: expected %d, got %d, groupID=%s, index=%d, requestID=%s",
			pollResp.FragmentSize, len(fragmentData), pollResp.FragmentGroupID, pollResp.FragmentIndex, requestID)
		return fmt.Errorf("fragment size mismatch: expected %d, got %d", pollResp.FragmentSize, len(fragmentData))
	}

	// 如果是分片，需要重组
	if isFragment {
		return sp.handleMultiFragment(pollResp, fragmentData, requestID)
	} else {
		return sp.handleSingleFragment(pollResp, fragmentData, requestID)
	}
}

// handleMultiFragment 处理多分片数据
func (sp *StreamProcessor) handleMultiFragment(pollResp FragmentResponse, fragmentData []byte, requestID string) error {
	// 添加到分片重组器
	group, err := sp.fragmentReassembler.AddFragment(
		pollResp.FragmentGroupID,
		pollResp.OriginalSize,
		pollResp.FragmentSize,
		pollResp.FragmentIndex,
		pollResp.TotalFragments,
		pollResp.SequenceNumber,
		fragmentData,
	)
	if err != nil {
		corelog.Errorf("HTTPStreamProcessor: handleMultiFragment - failed to add fragment: %v, groupID=%s, index=%d, requestID=%s", err, pollResp.FragmentGroupID, pollResp.FragmentIndex, requestID)
		return fmt.Errorf("failed to add fragment: %w", err)
	}

	// 检查是否完整
	isComplete := group.IsComplete()
	if !isComplete {
		// 分片组不完整，继续等待更多分片
		return nil
	}

	// 分片组完整，检查是否可以按序列号顺序发送

	return sp.processCompleteGroups(requestID)
}

// handleSingleFragment 处理单分片数据
func (sp *StreamProcessor) handleSingleFragment(pollResp FragmentResponse, fragmentData []byte, requestID string) error {
	// 单分片数据（TotalFragments=1），也需要按序列号顺序发送
	// 添加到分片重组器，以便按序列号顺序处理
	_, err := sp.fragmentReassembler.AddFragment(
		pollResp.FragmentGroupID,
		pollResp.OriginalSize,
		pollResp.FragmentSize,
		pollResp.FragmentIndex,
		pollResp.TotalFragments,
		pollResp.SequenceNumber,
		fragmentData,
	)
	if err != nil {
		corelog.Errorf("HTTPStreamProcessor: handleSingleFragment - failed to add single fragment: %v, groupID=%s, requestID=%s", err, pollResp.FragmentGroupID, requestID)
		return fmt.Errorf("failed to add single fragment: %w", err)
	}

	// 单分片数据应该立即完整，检查是否可以按序列号顺序发送

	return sp.processCompleteGroups(requestID)
}

// processCompleteGroups 处理所有按序列号顺序的完整分片组
func (sp *StreamProcessor) processCompleteGroups(requestID string) error {
	// 继续检查是否有更多按序列号顺序的完整分片组
	for {
		nextGroup, found, err := sp.fragmentReassembler.GetNextCompleteGroup()
		if err != nil {
			corelog.Errorf("HTTPStreamProcessor: processCompleteGroups - failed to get next complete group: %v, requestID=%s", err, requestID)
			return fmt.Errorf("failed to get next complete group: %w", err)
		}
		if !found {
			// 没有更多完整的分片组
			return nil
		}

		// 重组并写入缓冲区
		if err := sp.writeReassembledGroup(nextGroup, requestID); err != nil {
			return err
		}

		// 移除分片组
		sp.fragmentReassembler.RemoveGroup(nextGroup.GroupID)
	}
}

// writeReassembledGroup 重组分片组并写入数据缓冲区
func (sp *StreamProcessor) writeReassembledGroup(nextGroup *FragmentGroup, requestID string) error {
	reassembledData, err := nextGroup.Reassemble()
	if err != nil {
		corelog.Errorf("HTTPStreamProcessor: writeReassembledGroup - failed to reassemble: %v, groupID=%s, requestID=%s", err, nextGroup.GroupID, requestID)
		sp.fragmentReassembler.RemoveGroup(nextGroup.GroupID)
		return fmt.Errorf("failed to reassemble: %w", err)
	}

	// 验证重组后的数据大小
	if len(reassembledData) != nextGroup.OriginalSize {
		corelog.Errorf("HTTPStreamProcessor: writeReassembledGroup - reassembled size mismatch: expected %d, got %d, groupID=%s, requestID=%s",
			nextGroup.OriginalSize, len(reassembledData), nextGroup.GroupID, requestID)
		sp.fragmentReassembler.RemoveGroup(nextGroup.GroupID)
		return fmt.Errorf("reassembled size mismatch: expected %d, got %d", nextGroup.OriginalSize, len(reassembledData))
	}

	// 写入数据缓冲区
	sp.dataBufMu.Lock()
	if sp.dataBuffer.Len()+len(reassembledData) <= maxBufferSize {
		_, err := sp.dataBuffer.Write(reassembledData)
		if err != nil {
			corelog.Errorf("HTTPStreamProcessor: writeReassembledGroup - failed to write to data buffer: %v, requestID=%s", err, requestID)
			sp.dataBufMu.Unlock()
			return fmt.Errorf("failed to write to data buffer: %w", err)
		}
	} else {
		corelog.Errorf("HTTPStreamProcessor: writeReassembledGroup - data buffer full, dropping %d bytes, buffer size=%d, requestID=%s", len(reassembledData), sp.dataBuffer.Len(), requestID)
	}
	sp.dataBufMu.Unlock()

	return nil
}
