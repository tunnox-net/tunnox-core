package httppoll

import (
	"fmt"
	"sync"
	"time"

	"tunnox-core/internal/utils"
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
	Fragments      []*Fragment // 分片列表（按index排序）
	ReceivedCount  int         // 已接收分片数
	CreatedTime    time.Time   // 创建时间
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
		utils.Warnf("FragmentGroup[%s]: fragment %d already exists, ignoring duplicate", fg.GroupID, index)
		return nil // 忽略重复分片
	}

	// 检查大小
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

	utils.Infof("FragmentGroup[%s]: added fragment %d/%d (size=%d, received=%d/%d)",
		fg.GroupID, index, fg.TotalFragments, size, fg.ReceivedCount, fg.TotalFragments)

	return nil
}

// Reassemble 重组数据
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

	utils.Infof("FragmentGroup[%s]: reassembled %d bytes from %d fragments", fg.GroupID, len(result), fg.TotalFragments)
	return result, nil
}

// FragmentReassembler 分片重组管理器
type FragmentReassembler struct {
	groups map[string]*FragmentGroup
	mu     sync.RWMutex
}

// NewFragmentReassembler 创建分片重组管理器
func NewFragmentReassembler() *FragmentReassembler {
	reassembler := &FragmentReassembler{
		groups: make(map[string]*FragmentGroup),
	}

	// 启动清理协程
	go reassembler.cleanupLoop()

	return reassembler
}

// AddFragment 添加分片
func (fr *FragmentReassembler) AddFragment(groupID string, originalSize int, fragmentSize int, fragmentIndex int, totalFragments int, data []byte) (*FragmentGroup, error) {
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
			Fragments:      make([]*Fragment, totalFragments),
			CreatedTime:    time.Now(),
		}
		fr.groups[groupID] = group
		utils.Infof("FragmentReassembler: created new fragment group, groupID=%s, originalSize=%d, totalFragments=%d", groupID, originalSize, totalFragments)
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
	}

	fr.mu.Unlock()

	// 添加分片
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

// RemoveGroup 移除分片组
func (fr *FragmentReassembler) RemoveGroup(groupID string) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	delete(fr.groups, groupID)
	utils.Debugf("FragmentReassembler: removed fragment group, groupID=%s", groupID)
}

// cleanupExpiredLocked 清理过期的分片组（需要持有锁）
func (fr *FragmentReassembler) cleanupExpiredLocked() {
	now := time.Now()
	for groupID, group := range fr.groups {
		if now.Sub(group.CreatedTime) > FragmentGroupTimeout {
			delete(fr.groups, groupID)
			utils.Warnf("FragmentReassembler: removed expired fragment group, groupID=%s, age=%v", groupID, now.Sub(group.CreatedTime))
		}
	}
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
	lastFragmentSize := dataSize % MaxFragmentSize
	if lastFragmentSize > 0 && lastFragmentSize < MinFragmentSize && totalFragments > 1 {
		totalFragments--
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

