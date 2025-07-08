package generators

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/utils"
)

const (
	// 分段配置
	SegmentSize       = 1000000 // 每段100万个ID
	TotalSegments     = 90      // 总段数：90000000 / 1000000
	BitsPerUint64     = 64
	Uint64sPerSegment = SegmentSize / BitsPerUint64 // 15625

	// 存储键
	KeyClientIDSegments = "client_id_segments"
	KeySegmentStats     = "client_id_segment_stats"
	KeyUsedIDCount      = "client_id_used_count"

	// 最大重试次数
	MaxSegmentAttempts = 20
)

// SegmentBitmap 分段位图
type SegmentBitmap struct {
	segmentID int      // 段ID (0-89)
	startID   int64    // 段起始ID
	endID     int64    // 段结束ID
	bitmap    []uint64 // 位图数据
	usedCount int      // 已使用数量
	storage   storages.Storage
	mu        sync.RWMutex
}

// NewSegmentBitmap 创建新的分段位图
func NewSegmentBitmap(segmentID int, storage storages.Storage) *SegmentBitmap {
	startID := ClientIDMin + int64(segmentID*SegmentSize)
	endID := startID + int64(SegmentSize) - 1
	if endID > ClientIDMax {
		endID = ClientIDMax
	}

	return &SegmentBitmap{
		segmentID: segmentID,
		startID:   startID,
		endID:     endID,
		bitmap:    make([]uint64, Uint64sPerSegment),
		storage:   storage,
	}
}

// loadFromStorage 从存储加载位图数据
func (s *SegmentBitmap) loadFromStorage() error {
	data, err := s.storage.GetHash(KeyClientIDSegments, fmt.Sprintf("segment_%d", s.segmentID))
	if err != nil {
		// 段不存在，使用空位图
		return nil
	}

	// 解析位图数据
	if bitmapData, ok := data.(string); ok {
		return s.deserializeBitmap(bitmapData)
	}

	return fmt.Errorf("invalid bitmap data type")
}

// saveToStorage 保存位图数据到存储
func (s *SegmentBitmap) saveToStorage() error {
	bitmapData := s.serializeBitmap()

	return s.storage.SetHash(KeyClientIDSegments, fmt.Sprintf("segment_%d", s.segmentID), bitmapData)
}

// serializeBitmap 序列化位图数据
func (s *SegmentBitmap) serializeBitmap() string {
	data := make([]byte, len(s.bitmap)*8)
	for i, word := range s.bitmap {
		binary.LittleEndian.PutUint64(data[i*8:], word)
	}
	return string(data)
}

// deserializeBitmap 反序列化位图数据
func (s *SegmentBitmap) deserializeBitmap(data string) error {
	if len(data) != len(s.bitmap)*8 {
		return fmt.Errorf("invalid bitmap data length")
	}

	bytes := []byte(data)
	for i := range s.bitmap {
		s.bitmap[i] = binary.LittleEndian.Uint64(bytes[i*8 : (i+1)*8])
	}

	// 重新计算已使用数量
	s.recalculateUsedCount()
	return nil
}

// recalculateUsedCount 重新计算已使用数量
func (s *SegmentBitmap) recalculateUsedCount() {
	count := 0
	for _, word := range s.bitmap {
		// 计算位图中1的个数
		for word != 0 {
			count += int(word & 1)
			word >>= 1
		}
	}
	s.usedCount = count
}

// setBit 设置位
func (s *SegmentBitmap) setBit(id int64) {
	if id < s.startID || id > s.endID {
		return // 忽略超出范围的ID
	}
	offset := (id - s.startID) / BitsPerUint64
	bit := (id - s.startID) % BitsPerUint64
	s.bitmap[offset] |= (1 << bit)
	s.usedCount++
}

// clearBit 清除位
func (s *SegmentBitmap) clearBit(id int64) {
	if id < s.startID || id > s.endID {
		return // 忽略超出范围的ID
	}
	offset := (id - s.startID) / BitsPerUint64
	bit := (id - s.startID) % BitsPerUint64
	s.bitmap[offset] &^= (1 << bit)
	s.usedCount--
}

// isBitSet 检查位是否已设置
func (s *SegmentBitmap) isBitSet(id int64) bool {
	if id < s.startID || id > s.endID {
		return false // 超出范围的ID视为未使用
	}
	offset := (id - s.startID) / BitsPerUint64
	bit := (id - s.startID) % BitsPerUint64
	return (s.bitmap[offset] & (1 << bit)) != 0
}

// findUnusedPosition 查找未使用位置
func (s *SegmentBitmap) findUnusedPosition() (int64, bool) {
	for i, word := range s.bitmap {
		if word != 0xFFFFFFFFFFFFFFFF { // 不是全1
			for j := 0; j < BitsPerUint64; j++ {
				if (word & (1 << j)) == 0 {
					id := s.startID + int64(i*BitsPerUint64+j)
					if id <= s.endID {
						return id, true
					}
				}
			}
		}
	}
	return 0, false
}

// getUsageRate 获取使用率
func (s *SegmentBitmap) getUsageRate() float64 {
	totalBits := s.endID - s.startID + 1
	return float64(s.usedCount) / float64(totalBits)
}

// OptimizedClientIDGenerator 优化的客户端ID生成器
type OptimizedClientIDGenerator struct {
	storage      storages.Storage
	segments     []*SegmentBitmap
	segmentStats map[int]float64
	mu           sync.RWMutex
	utils.Dispose
}

// NewOptimizedClientIDGenerator 创建优化的客户端ID生成器
func NewOptimizedClientIDGenerator(storage storages.Storage, parentCtx context.Context) *OptimizedClientIDGenerator {
	generator := &OptimizedClientIDGenerator{
		storage:      storage,
		segments:     make([]*SegmentBitmap, TotalSegments),
		segmentStats: make(map[int]float64),
	}

	generator.SetCtx(parentCtx, generator.onClose)

	// 初始化所有段
	for i := 0; i < TotalSegments; i++ {
		generator.segments[i] = NewSegmentBitmap(i, storage)
	}

	// 加载段数据
	go generator.loadAllSegments()

	return generator
}

// onClose 资源清理回调
func (g *OptimizedClientIDGenerator) onClose() {
	utils.Infof("Optimized client ID generator resources cleaned up")

	// 保存所有段数据
	g.saveAllSegments()
}

// loadAllSegments 加载所有段数据
func (g *OptimizedClientIDGenerator) loadAllSegments() {
	for i, segment := range g.segments {
		if err := segment.loadFromStorage(); err != nil {
			utils.Errorf("Failed to load segment %d: %v", i, err)
		}
		g.updateSegmentStats(i, segment.getUsageRate())
	}

	utils.Infof("Loaded all segment data")
}

// saveAllSegments 保存所有段数据
func (g *OptimizedClientIDGenerator) saveAllSegments() {
	for i, segment := range g.segments {
		if err := segment.saveToStorage(); err != nil {
			utils.Errorf("Failed to save segment %d: %v", i, err)
		}
	}

	utils.Infof("Saved all segment data")
}

// updateSegmentStats 更新段统计信息
func (g *OptimizedClientIDGenerator) updateSegmentStats(segmentID int, usageRate float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.segmentStats[segmentID] = usageRate
}

// selectSegment 智能选择段
func (g *OptimizedClientIDGenerator) selectSegment() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// 计算权重
	var totalWeight float64
	weights := make([]float64, TotalSegments)

	for i := 0; i < TotalSegments; i++ {
		usageRate := g.segmentStats[i]
		// 权重 = 1 / (使用率 + 0.1)，使用率越低权重越高
		weight := 1.0 / (usageRate + 0.1)
		weights[i] = weight
		totalWeight += weight
	}

	// 随机选择
	r := rand.Float64() * totalWeight
	currentWeight := 0.0

	for i := 0; i < TotalSegments; i++ {
		currentWeight += weights[i]
		if r <= currentWeight {
			return i
		}
	}

	// 兜底：返回第一个段
	return 0
}

// GenerateClientID 生成客户端ID
func (g *OptimizedClientIDGenerator) GenerateClientID() (int64, error) {
	for attempts := 0; attempts < MaxSegmentAttempts; attempts++ {
		// 智能选择段
		segmentID := g.selectSegment()
		segment := g.segments[segmentID]

		// 在段内查找未使用位置
		segment.mu.Lock()
		id, found := segment.findUnusedPosition()
		if found {
			// 标记为已使用
			segment.setBit(id)
			segment.mu.Unlock()

			// 更新统计信息
			g.updateSegmentStats(segmentID, segment.getUsageRate())

			// 同步保存段数据（避免创建过多goroutine）
			if err := segment.saveToStorage(); err != nil {
				utils.Errorf("Failed to save segment %d: %v", segmentID, err)
			}

			return id, nil
		}
		segment.mu.Unlock()

		// 如果当前段满了，更新统计信息并重试
		g.updateSegmentStats(segmentID, 1.0)
	}

	return 0, ErrIDExhausted
}

// ReleaseClientID 释放客户端ID
func (g *OptimizedClientIDGenerator) ReleaseClientID(clientID int64) error {
	// 检查ID是否在有效范围内
	if clientID < ClientIDMin || clientID > ClientIDMax {
		return fmt.Errorf("invalid client ID: %d", clientID)
	}

	// 计算段ID
	segmentID := int((clientID - ClientIDMin) / SegmentSize)
	if segmentID < 0 || segmentID >= TotalSegments {
		return fmt.Errorf("invalid client ID: %d", clientID)
	}

	segment := g.segments[segmentID]

	segment.mu.Lock()
	defer segment.mu.Unlock()

	// 检查ID是否已使用
	if !segment.isBitSet(clientID) {
		return fmt.Errorf("client ID %d is not in use", clientID)
	}

	// 清除位
	segment.clearBit(clientID)

	// 更新统计信息
	g.updateSegmentStats(segmentID, segment.getUsageRate())

	// 同步保存段数据（避免创建过多goroutine）
	if err := segment.saveToStorage(); err != nil {
		utils.Errorf("Failed to save segment %d: %v", segmentID, err)
	}

	return nil
}

// IsClientIDUsed 检查客户端ID是否已使用
func (g *OptimizedClientIDGenerator) IsClientIDUsed(clientID int64) (bool, error) {
	// 检查ID是否在有效范围内
	if clientID < ClientIDMin || clientID > ClientIDMax {
		return false, fmt.Errorf("invalid client ID: %d", clientID)
	}

	// 计算段ID
	segmentID := int((clientID - ClientIDMin) / SegmentSize)
	if segmentID < 0 || segmentID >= TotalSegments {
		return false, fmt.Errorf("invalid client ID: %d", clientID)
	}

	segment := g.segments[segmentID]

	segment.mu.RLock()
	defer segment.mu.RUnlock()

	return segment.isBitSet(clientID), nil
}

// GetUsedCount 获取已使用的ID数量
func (g *OptimizedClientIDGenerator) GetUsedCount() int {
	total := 0
	for _, segment := range g.segments {
		segment.mu.RLock()
		total += segment.usedCount
		segment.mu.RUnlock()
	}
	return total
}

// GetSegmentStats 获取段统计信息
func (g *OptimizedClientIDGenerator) GetSegmentStats() map[int]float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stats := make(map[int]float64)
	for k, v := range g.segmentStats {
		stats[k] = v
	}
	return stats
}
