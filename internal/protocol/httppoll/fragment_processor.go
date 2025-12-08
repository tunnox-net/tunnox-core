package httppoll

import (
	"encoding/base64"
	coreErrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/utils"
)

// FragmentProcessor 分片处理器接口（统一服务器端和客户端的分片处理逻辑）
// 职责：处理分片数据的接收、重组和分发
type FragmentProcessor interface {
	// ProcessFragment 处理单个分片
	// 返回：是否已完整、重组后的数据（如果完整且可以立即返回）、错误
	// 注意：对于按序列号顺序的处理器，即使分片组完整，也可能返回 nil（需要等待序列号顺序）
	ProcessFragment(
		groupID string,
		originalSize int,
		fragmentSize int,
		fragmentIndex int,
		totalFragments int,
		sequenceNumber int64,
		fragmentData []byte,
	) (isComplete bool, reassembledData []byte, err error)

	// GetNextReassembledData 获取下一个按序列号顺序的重组数据（仅用于需要序列号顺序的场景）
	// 返回：重组后的数据、是否找到、错误
	// 注意：只有序列号匹配的分片组才会被返回
	GetNextReassembledData() ([]byte, bool, error)
}

// OrderedFragmentProcessor 按序列号顺序处理分片的处理器（用于客户端）
// 确保数据包按序列号顺序处理，避免乱序问题
type OrderedFragmentProcessor struct {
	reassembler *FragmentReassembler
}

// NewOrderedFragmentProcessor 创建按序列号顺序的分片处理器
func NewOrderedFragmentProcessor(reassembler *FragmentReassembler) *OrderedFragmentProcessor {
	return &OrderedFragmentProcessor{
		reassembler: reassembler,
	}
}

// ProcessFragment 处理单个分片（按序列号顺序）
func (p *OrderedFragmentProcessor) ProcessFragment(
	groupID string,
	originalSize int,
	fragmentSize int,
	fragmentIndex int,
	totalFragments int,
	sequenceNumber int64,
	fragmentData []byte,
) (bool, []byte, error) {
	// 添加到分片重组器
	group, err := p.reassembler.AddFragment(
		groupID,
		originalSize,
		fragmentSize,
		fragmentIndex,
		totalFragments,
		sequenceNumber,
		fragmentData,
	)
	if err != nil {
		return false, nil, coreErrors.Wrap(err, coreErrors.ErrorTypeProtocol, "failed to add fragment")
	}

	// 检查是否完整（但不立即重组，等待按序列号顺序处理）
	isComplete := group.IsComplete()
	if !isComplete {
		return false, nil, nil
	}

	// 分片组完整，但需要按序列号顺序处理
	// 通过 ProcessCompleteGroups() 方法按序列号顺序获取并重组
	return true, nil, nil
}

// GetNextReassembledData 获取下一个按序列号顺序的重组数据
func (p *OrderedFragmentProcessor) GetNextReassembledData() ([]byte, bool, error) {
	nextGroup, found, err := p.reassembler.GetNextCompleteGroup()
	if err != nil {
		return nil, false, coreErrors.Wrap(err, coreErrors.ErrorTypeProtocol, "failed to get next complete group")
	}
	if !found {
		return nil, false, nil
	}

	// 重组分片组（Reassemble 内部会检查 reassembled 标志）
	reassembledData, err := nextGroup.Reassemble()
	if err != nil {
		utils.Errorf("OrderedFragmentProcessor: failed to reassemble group %s: %v", nextGroup.GroupID, err)
		p.reassembler.RemoveGroup(nextGroup.GroupID)
		return nil, false, coreErrors.Wrap(err, coreErrors.ErrorTypeProtocol, "failed to reassemble")
	}

	// 验证重组后的数据大小
	if len(reassembledData) != nextGroup.OriginalSize {
		utils.Errorf("OrderedFragmentProcessor: reassembled size mismatch: expected %d, got %d, groupID=%s",
			nextGroup.OriginalSize, len(reassembledData), nextGroup.GroupID)
		p.reassembler.RemoveGroup(nextGroup.GroupID)
		return nil, false, coreErrors.Newf(coreErrors.ErrorTypeProtocol,
			"reassembled size mismatch: expected %d, got %d", nextGroup.OriginalSize, len(reassembledData))
	}

	// 移除分片组
	p.reassembler.RemoveGroup(nextGroup.GroupID)

	return reassembledData, true, nil
}

// ImmediateFragmentProcessor 立即处理分片的处理器（用于服务器端）
// 不需要按序列号顺序，因为服务器端接收的数据顺序由客户端保证
type ImmediateFragmentProcessor struct {
	reassembler *FragmentReassembler
}

// NewImmediateFragmentProcessor 创建立即处理的分片处理器
func NewImmediateFragmentProcessor(reassembler *FragmentReassembler) *ImmediateFragmentProcessor {
	return &ImmediateFragmentProcessor{
		reassembler: reassembler,
	}
}

// ProcessFragment 处理单个分片（立即重组，不等待序列号顺序）
func (p *ImmediateFragmentProcessor) ProcessFragment(
	groupID string,
	originalSize int,
	fragmentSize int,
	fragmentIndex int,
	totalFragments int,
	sequenceNumber int64,
	fragmentData []byte,
) (bool, []byte, error) {
	// 添加到分片重组器
	group, err := p.reassembler.AddFragment(
		groupID,
		originalSize,
		fragmentSize,
		fragmentIndex,
		totalFragments,
		sequenceNumber,
		fragmentData,
	)
	if err != nil {
		return false, nil, coreErrors.Wrap(err, coreErrors.ErrorTypeProtocol, "failed to add fragment")
	}

	// 使用原子操作检查是否完整并重组（避免竞态条件）
	reassembledData, isComplete, err := group.IsCompleteAndReassemble()
	if err != nil {
		return false, nil, coreErrors.Wrap(err, coreErrors.ErrorTypeProtocol, "failed to reassemble")
	}

	if isComplete {
		// 验证重组后的数据大小
		if len(reassembledData) != originalSize {
			p.reassembler.RemoveGroup(groupID)
			return false, nil, coreErrors.Newf(coreErrors.ErrorTypeProtocol,
				"reassembled size mismatch: expected %d, got %d", originalSize, len(reassembledData))
		}

		// 移除分片组（延迟移除，确保其他 goroutine 不会重复处理）
		p.reassembler.RemoveGroup(groupID)
		return true, reassembledData, nil
	}

	// 分片组不完整或已被其他 goroutine 重组
	return false, nil, nil
}

// GetNextReassembledData 立即处理器不需要按序列号顺序处理
func (p *ImmediateFragmentProcessor) GetNextReassembledData() ([]byte, bool, error) {
	// 立即处理器不需要按序列号顺序处理，所有分片组在 ProcessFragment 中立即处理
	return nil, false, nil
}

// ProcessFragmentFromResponse 从 FragmentResponse 处理分片（统一入口）
// 职责：解码、验证、处理分片
func ProcessFragmentFromResponse(
	processor FragmentProcessor,
	resp FragmentResponse,
) (isComplete bool, reassembledData []byte, err error) {
	// 解码 Base64 数据
	fragmentData, err := base64.StdEncoding.DecodeString(resp.Data)
	if err != nil {
		return false, nil, coreErrors.Wrap(err, coreErrors.ErrorTypeProtocol, "failed to decode fragment data")
	}

	// 验证解码后的数据长度是否与 FragmentSize 匹配
	if len(fragmentData) != resp.FragmentSize {
		return false, nil, coreErrors.Newf(coreErrors.ErrorTypeProtocol,
			"fragment size mismatch: expected %d, got %d", resp.FragmentSize, len(fragmentData))
	}

	// 判断是否为分片：total_fragments > 1
	isFragment := resp.TotalFragments > 1

	if isFragment {
		// 多分片，需要重组
		return processor.ProcessFragment(
			resp.FragmentGroupID,
			resp.OriginalSize,
			resp.FragmentSize,
			resp.FragmentIndex,
			resp.TotalFragments,
			resp.SequenceNumber,
			fragmentData,
		)
	}

	// 单分片（TotalFragments=1），对于立即处理器，直接返回数据
	// 对于有序处理器，仍然需要通过重组器处理（以保持序列号顺序）
	return processor.ProcessFragment(
		resp.FragmentGroupID,
		resp.OriginalSize,
		resp.FragmentSize,
		resp.FragmentIndex,
		resp.TotalFragments,
		resp.SequenceNumber,
		fragmentData,
	)
}

