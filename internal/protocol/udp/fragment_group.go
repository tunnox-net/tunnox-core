package udp

import (
	"fmt"
	"time"
)

// FragmentGroupKey 分片组键
type FragmentGroupKey struct {
	SessionID uint32
	StreamID  uint32
	PacketSeq uint32
}

// FragmentGroup 负责管理某个 PacketSeq 的所有分片。
type FragmentGroup struct {
	Key            FragmentGroupKey
	TotalFragments int
	ReceivedCount  int
	OriginalSize   int

	Fragments      [][]byte   // len == TotalFragments，按 FragSeq 下标存储
	CreatedAt      time.Time
	LastAccessTime time.Time
}

// NewFragmentGroup 创建新的分片组
func NewFragmentGroup(key FragmentGroupKey, totalFragments int, originalSize int) *FragmentGroup {
	now := time.Now()
	return &FragmentGroup{
		Key:            key,
		TotalFragments: totalFragments,
		ReceivedCount:  0,
		OriginalSize:   originalSize,
		Fragments:      make([][]byte, totalFragments),
		CreatedAt:      now,
		LastAccessTime: now,
	}
}

// AddFragment 写入一个分片。
// - fragSeq: 当前分片序号（0..TotalFragments-1）
// - data: 该分片数据（调用方需要自行拷贝或保证后续不修改）
func (g *FragmentGroup) AddFragment(fragSeq int, data []byte) error {
	if fragSeq < 0 || fragSeq >= g.TotalFragments {
		return fmt.Errorf("invalid fragSeq: %d, expected 0..%d", fragSeq, g.TotalFragments-1)
	}

	// 重复分片忽略
	if g.Fragments[fragSeq] != nil {
		return nil
	}

	// 拷贝数据，避免外部修改
	fragData := make([]byte, len(data))
	copy(fragData, data)
	g.Fragments[fragSeq] = fragData
	g.ReceivedCount++
	g.LastAccessTime = time.Now()

	return nil
}

// IsComplete 检查是否所有分片都已收到
func (g *FragmentGroup) IsComplete() bool {
	return g.ReceivedCount == g.TotalFragments
}

// Reassemble 在完整时按 FragSeq 顺序拼接为原始 payload。
// 长度不符 OriginalSize 时返回 error。
func (g *FragmentGroup) Reassemble() ([]byte, error) {
	if !g.IsComplete() {
		return nil, fmt.Errorf("fragment group not complete: %d/%d", g.ReceivedCount, g.TotalFragments)
	}

	result := make([]byte, 0, g.OriginalSize)
	for i := 0; i < g.TotalFragments; i++ {
		if g.Fragments[i] == nil {
			return nil, fmt.Errorf("missing fragment %d", i)
		}
		result = append(result, g.Fragments[i]...)
	}

	if len(result) != g.OriginalSize {
		return nil, fmt.Errorf("reassembled size mismatch: expected %d, got %d", g.OriginalSize, len(result))
	}

	return result, nil
}

