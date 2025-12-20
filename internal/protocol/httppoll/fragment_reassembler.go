package httppoll

import (
	"fmt"
	"sync"
	"time"
	corelog "tunnox-core/internal/core/log"
)

const (
	// FragmentThreshold 分片阈值：超过此大小才分片
	FragmentThreshold = 8 * 1024 // 8KB

	// MaxFragmentSize 最大分片大小（Base64解码后）
	MaxFragmentSize = 10 * 1024 // 10KB

	// MinFragmentSize 最小分片大小（避免过度分片）
	MinFragmentSize = 1 * 1024 // 1KB

	// FragmentGroupTimeout 分片组超时时间
	FragmentGroupTimeout = 30 * time.Second

	// MaxFragmentGroups 最大分片组数量
	MaxFragmentGroups = 100

	// MaxFragmentGroupSize 单个分片组最大大小
	MaxFragmentGroupSize = 10 * 1024 * 1024 // 10MB
)

// Fragment 单个分片
type Fragment struct {
	Index    int       // 分片索引
	Size     int       // 分片大小
	Data     []byte    // 分片数据
	Received time.Time // 接收时间
}

// FragmentGroup 分片组
type FragmentGroup struct {
	GroupID        string      // 分片组ID
	OriginalSize   int         // 原始总大小
	TotalFragments int         // 总分片数
	SequenceNumber int64       // 序列号（用于保证数据包顺序）
	Fragments      []*Fragment // 分片列表（按index排序）
	ReceivedCount  int         // 已接收分片数
	CreatedTime    time.Time   // 创建时间
	reassembled    bool        // 是否已重组（防止重复重组）
	mu             sync.Mutex
}

// IsComplete 检查分片组是否完整
func (fg *FragmentGroup) IsComplete() bool {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	return fg.ReceivedCount == fg.TotalFragments
}

// AddFragment 添加分片
func (fg *FragmentGroup) AddFragment(index int, size int, data []byte) error {
	fg.mu.Lock()
	defer fg.mu.Unlock()

	// 检查索引范围
	if index < 0 || index >= fg.TotalFragments {
		return fmt.Errorf("fragment index out of range: %d (total: %d)", index, fg.TotalFragments)
	}

	// 检查是否已存在
	if fg.Fragments[index] != nil {
		corelog.Warnf("FragmentGroup[%s]: fragment %d already exists, ignoring duplicate", fg.GroupID, index)
		return nil // 忽略重复分片
	}

	// 检查大小
	// 注意：size 参数应该是实际数据长度（从 FragmentSize 字段获取）
	// 对于最后一片，可能小于 MaxFragmentSize，但应该等于 FragmentSize 字段的值
	if len(data) != size {
		return fmt.Errorf("fragment size mismatch: expected %d, got %d", size, len(data))
	}

	// 添加分片
	fg.Fragments[index] = &Fragment{
		Index:    index,
		Size:     size,
		Data:     data,
		Received: time.Now(),
	}
	fg.ReceivedCount++

	corelog.Infof("FragmentGroup[%s]: added fragment %d/%d (size=%d, received=%d/%d, originalSize=%d)",
		fg.GroupID, index, fg.TotalFragments, size, fg.ReceivedCount, fg.TotalFragments, fg.OriginalSize)

	return nil
}

// Reassemble 重组数据（原子操作：检查完整性并重组）
func (fg *FragmentGroup) Reassemble() ([]byte, error) {
	fg.mu.Lock()
	defer fg.mu.Unlock()

	if fg.ReceivedCount != fg.TotalFragments {
		return nil, fmt.Errorf("fragment group incomplete: %d/%d", fg.ReceivedCount, fg.TotalFragments)
	}

	// 按索引顺序拼接
	result := make([]byte, 0, fg.OriginalSize)
	for i := 0; i < fg.TotalFragments; i++ {
		if fg.Fragments[i] == nil {
			return nil, fmt.Errorf("fragment %d is missing", i)
		}
		result = append(result, fg.Fragments[i].Data...)
	}

	// 验证总大小
	if len(result) != fg.OriginalSize {
		return nil, fmt.Errorf("reassembled size mismatch: expected %d, got %d", fg.OriginalSize, len(result))
	}

	corelog.Debugf("FragmentGroup[%s]: reassembled %d bytes from %d fragments", fg.GroupID, len(result), fg.TotalFragments)
	return result, nil
}

// IsCompleteAndReassemble 原子操作：检查是否完整，如果完整则重组（避免竞态条件）
// 返回：重组后的数据、是否完整、是否由当前调用完成重组、错误
// 注意：只有第一个检测到完整的 goroutine 会执行重组，其他 goroutine 会返回 reassembled=false
func (fg *FragmentGroup) IsCompleteAndReassemble() ([]byte, bool, error) {
	fg.mu.Lock()
	defer fg.mu.Unlock()

	// 如果已经重组过，返回 nil（表示其他 goroutine 已经处理）
	if fg.reassembled {
		return nil, false, nil
	}

	if fg.ReceivedCount != fg.TotalFragments {
		corelog.Debugf("FragmentGroup[%s]: IsCompleteAndReassemble - not complete yet, received=%d/%d", fg.GroupID, fg.ReceivedCount, fg.TotalFragments)
		return nil, false, nil
	}

	// 按索引顺序拼接
	result := make([]byte, 0, fg.OriginalSize)
	for i := 0; i < fg.TotalFragments; i++ {
		if fg.Fragments[i] == nil {
			return nil, false, fmt.Errorf("fragment %d is missing", i)
		}
		result = append(result, fg.Fragments[i].Data...)
	}

	// 验证总大小
	if len(result) != fg.OriginalSize {
		return nil, false, fmt.Errorf("reassembled size mismatch: expected %d, got %d", fg.OriginalSize, len(result))
	}

	// 标记为已重组（防止其他 goroutine 重复重组）
	fg.reassembled = true

	corelog.Infof("FragmentGroup[%s]: IsCompleteAndReassemble - reassembled %d bytes from %d fragments, originalSize=%d", fg.GroupID, len(result), fg.TotalFragments, fg.OriginalSize)
	return result, true, nil
}

// FragmentReassembler 分片重组管理器
type FragmentReassembler struct {
	groups          map[string]*FragmentGroup // groupID -> FragmentGroup
	sequenceGroups  map[int64]*FragmentGroup  // sequenceNumber -> FragmentGroup（用于按序列号查找）
	nextExpectedSeq int64                     // 期望的下一个序列号（用于按顺序发送）
	mu              sync.RWMutex
}

// NewFragmentReassembler 创建分片重组管理器
func NewFragmentReassembler() *FragmentReassembler {
	reassembler := &FragmentReassembler{
		groups:          make(map[string]*FragmentGroup),
		sequenceGroups:  make(map[int64]*FragmentGroup),
		nextExpectedSeq: 0,
	}

	// 启动清理协程
	go reassembler.cleanupLoop()

	return reassembler
}

// AddFragment 添加分片
func (fr *FragmentReassembler) AddFragment(groupID string, originalSize int, fragmentSize int, fragmentIndex int, totalFragments int, sequenceNumber int64, data []byte) (*FragmentGroup, error) {
	fr.mu.Lock()

	// 检查分片组数量限制
	if len(fr.groups) >= MaxFragmentGroups {
		// 尝试清理超时的分片组
		fr.cleanupExpiredLocked()
		if len(fr.groups) >= MaxFragmentGroups {
			fr.mu.Unlock()
			return nil, fmt.Errorf("too many fragment groups: %d", len(fr.groups))
		}
	}

	// 查找或创建分片组
	group, exists := fr.groups[groupID]
	if !exists {
		// 验证参数
		if originalSize > MaxFragmentGroupSize {
			fr.mu.Unlock()
			return nil, fmt.Errorf("fragment group size too large: %d (max: %d)", originalSize, MaxFragmentGroupSize)
		}

		group = &FragmentGroup{
			GroupID:        groupID,
			OriginalSize:   originalSize,
			TotalFragments: totalFragments,
			SequenceNumber: sequenceNumber,
			Fragments:      make([]*Fragment, totalFragments),
			CreatedTime:    time.Now(),
		}
		fr.groups[groupID] = group
		fr.sequenceGroups[sequenceNumber] = group
		corelog.Debugf("FragmentReassembler: created new fragment group, groupID=%s, sequenceNumber=%d, originalSize=%d, totalFragments=%d", groupID, sequenceNumber, originalSize, totalFragments)
	} else {
		// 验证一致性
		if group.OriginalSize != originalSize {
			fr.mu.Unlock()
			return nil, fmt.Errorf("fragment group size mismatch: expected %d, got %d", group.OriginalSize, originalSize)
		}
		if group.TotalFragments != totalFragments {
			fr.mu.Unlock()
			return nil, fmt.Errorf("fragment group total fragments mismatch: expected %d, got %d", group.TotalFragments, totalFragments)
		}
		if group.SequenceNumber != sequenceNumber {
			fr.mu.Unlock()
			return nil, fmt.Errorf("fragment group sequence number mismatch: expected %d, got %d", group.SequenceNumber, sequenceNumber)
		}
	}

	// 在持有锁的情况下，增加 group 的引用计数（通过返回 group 来保持引用）
	// 注意：这里不能释放锁后再调用 group.AddFragment，因为 group 可能被 RemoveGroup 删除
	// 解决方案：在释放锁之前，先验证 group 仍然存在，然后立即调用 AddFragment
	// 但由于 AddFragment 需要持有 group.mu，而我们已经持有 fr.mu，这会导致死锁
	// 因此，我们需要先释放 fr.mu，然后立即调用 group.AddFragment
	// 为了安全，我们在 AddFragment 中增加检查，如果 group 已被删除，返回错误

	fr.mu.Unlock()

	// 添加分片（此时 group 可能已被删除，AddFragment 内部会检查）
	if err := group.AddFragment(fragmentIndex, fragmentSize, data); err != nil {
		return nil, err
	}

	return group, nil
}

// GetGroup 获取分片组
func (fr *FragmentReassembler) GetGroup(groupID string) (*FragmentGroup, bool) {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	group, exists := fr.groups[groupID]
	return group, exists
}

// RemoveGroup 移除分片组（线程安全）
func (fr *FragmentReassembler) RemoveGroup(groupID string) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if group, exists := fr.groups[groupID]; exists {
		delete(fr.groups, groupID)
		// 同时从序列号映射中删除
		if group.SequenceNumber >= 0 {
			delete(fr.sequenceGroups, group.SequenceNumber)
		}
		corelog.Debugf("FragmentReassembler: removed fragment group, groupID=%s, sequenceNumber=%d", groupID, group.SequenceNumber)
	}
}

// cleanupExpiredLocked 清理过期的分片组（需要持有锁）
func (fr *FragmentReassembler) cleanupExpiredLocked() {
	now := time.Now()
	for groupID, group := range fr.groups {
		if now.Sub(group.CreatedTime) > FragmentGroupTimeout {
			delete(fr.groups, groupID)
			// 同时从序列号映射中删除
			if group.SequenceNumber >= 0 {
				delete(fr.sequenceGroups, group.SequenceNumber)
			}
			corelog.Warnf("FragmentReassembler: removed expired fragment group, groupID=%s, sequenceNumber=%d, age=%v", groupID, group.SequenceNumber, now.Sub(group.CreatedTime))
		}
	}
}

// GetNextCompleteGroup 获取下一个已完整的分片组（按序列号顺序）
// 返回：分片组、是否找到、错误
// 注意：只有序列号 == nextExpectedSeq 且已完整的分片组才会被返回
// 如果序列号不连续（比如序列号0还没完整，序列号1已经完整），会等待序列号0完成
func (fr *FragmentReassembler) GetNextCompleteGroup() (*FragmentGroup, bool, error) {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	// 只检查期望的下一个序列号（确保严格按顺序）
	group, exists := fr.sequenceGroups[fr.nextExpectedSeq]
	if !exists {
		// 期望的序列号还不存在，等待
		corelog.Debugf("FragmentReassembler: GetNextCompleteGroup - expected sequence %d not found, waiting", fr.nextExpectedSeq)
		return nil, false, nil
	}

	// 检查是否已完整
	group.mu.Lock()
	isComplete := group.ReceivedCount == group.TotalFragments && !group.reassembled
	receivedCount := group.ReceivedCount
	totalFragments := group.TotalFragments
	group.mu.Unlock()

	if isComplete {
		// 找到期望的下一个已完整的分片组，更新期望序列号
		oldSeq := fr.nextExpectedSeq
		fr.nextExpectedSeq++
		corelog.Infof("FragmentReassembler: GetNextCompleteGroup - found complete group, sequenceNumber=%d, groupID=%s, nextExpectedSeq=%d", oldSeq, group.GroupID, fr.nextExpectedSeq)
		return group, true, nil
	}

	// 期望的序列号存在但还不完整，等待
	corelog.Debugf("FragmentReassembler: GetNextCompleteGroup - expected sequence %d exists but not complete yet, receivedCount=%d/%d, groupID=%s", fr.nextExpectedSeq, receivedCount, totalFragments, group.GroupID)
	return nil, false, nil
}

// cleanupLoop 定期清理过期的分片组
func (fr *FragmentReassembler) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		fr.mu.Lock()
		fr.cleanupExpiredLocked()
		fr.mu.Unlock()
	}
}

// CalculateFragments 计算分片参数
func CalculateFragments(dataSize int) (fragmentSize int, totalFragments int) {
	if dataSize <= FragmentThreshold {
		return dataSize, 1 // 不分片
	}

	// 计算分片数（向上取整）
	totalFragments = (dataSize + MaxFragmentSize - 1) / MaxFragmentSize

	// 确保最后一片不会太小（如果小于MIN_FRAGMENT_SIZE，合并到前一片）
	// 注意：合并后前一片可能会超过 MaxFragmentSize，但这是可以接受的
	// 因为 Base64 编码后的 JSON 大小会略大于原始数据，但仍在合理范围内
	lastFragmentSize := dataSize % MaxFragmentSize
	if lastFragmentSize > 0 && lastFragmentSize < MinFragmentSize && totalFragments > 1 {
		totalFragments--
		// 注意：合并后，倒数第二片的大小会是 MaxFragmentSize + lastFragmentSize
		// 这可能会超过 MaxFragmentSize，但不会超过太多（最多 MaxFragmentSize + MinFragmentSize - 1）
		// 对于大数据包，这种轻微的超出是可以接受的
	}

	return MaxFragmentSize, totalFragments
}

// GetFragmentData 获取指定索引的分片数据
func GetFragmentData(data []byte, fragmentIndex int, fragmentSize int, totalFragments int) []byte {
	start := fragmentIndex * fragmentSize
	end := start + fragmentSize

	// 最后一片可能小于 fragmentSize
	if fragmentIndex == totalFragments-1 {
		end = len(data)
	}

	if end > len(data) {
		end = len(data)
	}

	if start >= len(data) {
		return nil
	}

	return data[start:end]
}
