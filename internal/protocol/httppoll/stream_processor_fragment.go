package httppoll

import (
	coreErrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/utils"
)

// handleFragmentData 处理分片数据（从sendPollRequest中提取）
// 使用统一的分片处理接口，消除重复代码
func (sp *StreamProcessor) handleFragmentData(pollResp FragmentResponse, requestID string) error {
	// 确保 fragmentProcessor 已初始化（按序列号顺序处理）
	if sp.fragmentProcessor == nil {
		sp.fragmentProcessor = NewOrderedFragmentProcessor(sp.fragmentReassembler)
	}

	// 使用统一的分片处理器（按序列号顺序处理）
	isComplete, reassembledData, err := ProcessFragmentFromResponse(sp.fragmentProcessor, pollResp)
	if err != nil {
		utils.Errorf("HTTPStreamProcessor: handleFragmentData - failed to process fragment: %v, groupID=%s, requestID=%s",
			err, pollResp.FragmentGroupID, requestID)
		return coreErrors.Wrap(err, coreErrors.ErrorTypeProtocol, "failed to process fragment")
	}

	// 如果分片组完整且可以立即返回（单分片情况），直接写入缓冲区
	if isComplete && reassembledData != nil {
		return sp.writeReassembledData(reassembledData, requestID)
	}

	// 处理所有按序列号顺序的完整分片组（包括当前分片组和之前等待的分片组）
	return sp.processCompleteGroups(requestID)
}

// processCompleteGroups 处理所有按序列号顺序的完整分片组
func (sp *StreamProcessor) processCompleteGroups(requestID string) error {
	// 确保 fragmentProcessor 已初始化
	if sp.fragmentProcessor == nil {
		sp.fragmentProcessor = NewOrderedFragmentProcessor(sp.fragmentReassembler)
	}

	// 继续检查是否有更多按序列号顺序的完整分片组
	for {
		reassembledData, found, err := sp.fragmentProcessor.GetNextReassembledData()
		if err != nil {
			utils.Errorf("HTTPStreamProcessor: processCompleteGroups - failed to get next reassembled data: %v, requestID=%s", err, requestID)
			return coreErrors.Wrap(err, coreErrors.ErrorTypeProtocol, "failed to get next reassembled data")
		}
		if !found {
			// 没有更多按序列号顺序的完整分片组
			return nil
		}

		// 写入数据缓冲区
		if err := sp.writeReassembledData(reassembledData, requestID); err != nil {
			return err
		}
	}
}

// writeReassembledData 将重组后的数据写入数据缓冲区
func (sp *StreamProcessor) writeReassembledData(reassembledData []byte, requestID string) error {
	// 写入数据缓冲区
	sp.dataBufMu.Lock()
	defer sp.dataBufMu.Unlock()

	if sp.dataBuffer.Len()+len(reassembledData) <= maxBufferSize {
		_, err := sp.dataBuffer.Write(reassembledData)
		if err != nil {
			utils.Errorf("HTTPStreamProcessor: writeReassembledData - failed to write to data buffer: %v, requestID=%s", err, requestID)
			return coreErrors.Wrap(err, coreErrors.ErrorTypeStorage, "failed to write to data buffer")
		}
	} else {
		utils.Errorf("HTTPStreamProcessor: writeReassembledData - data buffer full, dropping %d bytes, buffer size=%d, requestID=%s",
			len(reassembledData), sp.dataBuffer.Len(), requestID)
	}

	return nil
}
