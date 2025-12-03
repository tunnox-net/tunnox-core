package httppoll

import (
	"fmt"
	"testing"
	"time"
)

func TestFragmentGroup_AddFragment(t *testing.T) {
	group := &FragmentGroup{
		GroupID:        "test-group",
		OriginalSize:   100,
		TotalFragments: 3,
		Fragments:      make([]*Fragment, 3),
	}

	// 添加第一个分片
	err := group.AddFragment(0, 33, make([]byte, 33))
	if err != nil {
		t.Fatalf("AddFragment failed: %v", err)
	}
	if group.ReceivedCount != 1 {
		t.Errorf("Expected ReceivedCount=1, got %d", group.ReceivedCount)
	}

	// 添加第二个分片
	err = group.AddFragment(1, 33, make([]byte, 33))
	if err != nil {
		t.Fatalf("AddFragment failed: %v", err)
	}
	if group.ReceivedCount != 2 {
		t.Errorf("Expected ReceivedCount=2, got %d", group.ReceivedCount)
	}

	// 添加最后一个分片
	err = group.AddFragment(2, 34, make([]byte, 34))
	if err != nil {
		t.Fatalf("AddFragment failed: %v", err)
	}
	if group.ReceivedCount != 3 {
		t.Errorf("Expected ReceivedCount=3, got %d", group.ReceivedCount)
	}
}

func TestFragmentGroup_AddFragment_IndexOutOfRange(t *testing.T) {
	group := &FragmentGroup{
		GroupID:        "test-group",
		OriginalSize:   100,
		TotalFragments: 3,
		Fragments:      make([]*Fragment, 3),
	}

	// 索引越界
	err := group.AddFragment(3, 10, make([]byte, 10))
	if err == nil {
		t.Error("Expected error for index out of range")
	}

	// 负数索引
	err = group.AddFragment(-1, 10, make([]byte, 10))
	if err == nil {
		t.Error("Expected error for negative index")
	}
}

func TestFragmentGroup_AddFragment_SizeMismatch(t *testing.T) {
	group := &FragmentGroup{
		GroupID:        "test-group",
		OriginalSize:   100,
		TotalFragments: 3,
		Fragments:      make([]*Fragment, 3),
	}

	// 大小不匹配
	err := group.AddFragment(0, 33, make([]byte, 30))
	if err == nil {
		t.Error("Expected error for size mismatch")
	}
}

func TestFragmentGroup_AddFragment_Duplicate(t *testing.T) {
	group := &FragmentGroup{
		GroupID:        "test-group",
		OriginalSize:   100,
		TotalFragments: 3,
		Fragments:      make([]*Fragment, 3),
	}

	// 添加第一个分片
	err := group.AddFragment(0, 33, make([]byte, 33))
	if err != nil {
		t.Fatalf("AddFragment failed: %v", err)
	}

	// 重复添加（应该被忽略）
	err = group.AddFragment(0, 33, make([]byte, 33))
	if err != nil {
		t.Errorf("Duplicate fragment should be ignored, got error: %v", err)
	}
	if group.ReceivedCount != 1 {
		t.Errorf("Expected ReceivedCount=1 after duplicate, got %d", group.ReceivedCount)
	}
}

func TestFragmentGroup_IsComplete(t *testing.T) {
	group := &FragmentGroup{
		GroupID:        "test-group",
		OriginalSize:   100,
		TotalFragments: 3,
		Fragments:      make([]*Fragment, 3),
	}

	// 初始状态应该不完整
	if group.IsComplete() {
		t.Error("Group should not be complete initially")
	}

	// 添加所有分片
	group.AddFragment(0, 33, make([]byte, 33))
	group.AddFragment(1, 33, make([]byte, 33))
	group.AddFragment(2, 34, make([]byte, 34))

	// 应该完整
	if !group.IsComplete() {
		t.Error("Group should be complete after adding all fragments")
	}
}

func TestFragmentGroup_Reassemble(t *testing.T) {
	group := &FragmentGroup{
		GroupID:        "test-group",
		OriginalSize:   100,
		TotalFragments: 3,
		Fragments:      make([]*Fragment, 3),
	}

	// 准备测试数据（确保长度正确）
	data1 := make([]byte, 33)
	copy(data1, []byte("fragment 1 data with 33 bytes"))
	data2 := make([]byte, 33)
	copy(data2, []byte("fragment 2 data with 33 bytes"))
	data3 := make([]byte, 34)
	copy(data3, []byte("fragment 3 data with 34 bytes"))

	// 添加分片
	group.AddFragment(0, 33, data1)
	group.AddFragment(1, 33, data2)
	group.AddFragment(2, 34, data3)

	// 重组
	result, err := group.Reassemble()
	if err != nil {
		t.Fatalf("Reassemble failed: %v", err)
	}

	// 验证结果
	expected := append(append(data1, data2...), data3...)
	if len(result) != len(expected) {
		t.Errorf("Reassembled size mismatch: expected %d, got %d", len(expected), len(result))
	}
	if string(result) != string(expected) {
		t.Error("Reassembled data does not match expected")
	}
}

func TestFragmentGroup_Reassemble_Incomplete(t *testing.T) {
	group := &FragmentGroup{
		GroupID:        "test-group",
		OriginalSize:   100,
		TotalFragments: 3,
		Fragments:      make([]*Fragment, 3),
	}

	// 只添加一个分片
	group.AddFragment(0, 33, make([]byte, 33))

	// 尝试重组（应该失败）
	_, err := group.Reassemble()
	if err == nil {
		t.Error("Expected error when reassembling incomplete group")
	}
}

func TestFragmentGroup_IsCompleteAndReassemble(t *testing.T) {
	group := &FragmentGroup{
		GroupID:        "test-group",
		OriginalSize:   100,
		TotalFragments: 3,
		Fragments:      make([]*Fragment, 3),
	}

	// 准备测试数据（确保长度正确）
	data1 := make([]byte, 33)
	copy(data1, []byte("fragment 1 data with 33 bytes"))
	data2 := make([]byte, 33)
	copy(data2, []byte("fragment 2 data with 33 bytes"))
	data3 := make([]byte, 34)
	copy(data3, []byte("fragment 3 data with 34 bytes"))

	// 添加分片
	group.AddFragment(0, 33, data1)
	group.AddFragment(1, 33, data2)
	group.AddFragment(2, 34, data3)

	// 原子检查和重组
	result, complete, err := group.IsCompleteAndReassemble()
	if err != nil {
		t.Fatalf("IsCompleteAndReassemble failed: %v", err)
	}
	if !complete {
		t.Error("Group should be complete")
	}
	if result == nil {
		t.Error("Result should not be nil")
	}

	// 再次调用应该返回 reassembled=false（已重组过）
	result2, complete2, err2 := group.IsCompleteAndReassemble()
	if err2 != nil {
		t.Fatalf("IsCompleteAndReassemble failed on second call: %v", err2)
	}
	if complete2 {
		t.Error("Second call should return complete=false (already reassembled)")
	}
	if result2 != nil {
		t.Error("Second call should return nil result")
	}
}

func TestFragmentReassembler_AddFragment(t *testing.T) {
	reassembler := NewFragmentReassembler()

	groupID := "test-group"
	originalSize := 100
	fragmentSize := 33
	totalFragments := 3

	// 添加第一个分片
	group, err := reassembler.AddFragment(groupID, originalSize, fragmentSize, 0, totalFragments, 0, make([]byte, fragmentSize))
	if err != nil {
		t.Fatalf("AddFragment failed: %v", err)
	}
	if group == nil {
		t.Fatal("Group should not be nil")
	}
	if group.ReceivedCount != 1 {
		t.Errorf("Expected ReceivedCount=1, got %d", group.ReceivedCount)
	}

	// 添加第二个分片
	group2, err := reassembler.AddFragment(groupID, originalSize, fragmentSize, 1, totalFragments, 0, make([]byte, fragmentSize))
	if err != nil {
		t.Fatalf("AddFragment failed: %v", err)
	}
	if group2 != group {
		t.Error("Should return same group for same groupID")
	}
	if group.ReceivedCount != 2 {
		t.Errorf("Expected ReceivedCount=2, got %d", group.ReceivedCount)
	}
}

func TestFragmentReassembler_AddFragment_SizeMismatch(t *testing.T) {
	reassembler := NewFragmentReassembler()

	groupID := "test-group"
	originalSize := 100
	fragmentSize := 33
	totalFragments := 3

	// 添加第一个分片
	reassembler.AddFragment(groupID, originalSize, fragmentSize, 0, totalFragments, 0, make([]byte, fragmentSize))

	// 尝试添加大小不匹配的分片
	_, err := reassembler.AddFragment(groupID, originalSize+10, fragmentSize, 1, totalFragments, 0, make([]byte, fragmentSize))
	if err == nil {
		t.Error("Expected error for size mismatch")
	}
}

func TestFragmentReassembler_GetGroup(t *testing.T) {
	reassembler := NewFragmentReassembler()

	groupID := "test-group"

	// 获取不存在的组
	_, exists := reassembler.GetGroup(groupID)
	if exists {
		t.Error("Group should not exist")
	}

	// 添加分片创建组
	reassembler.AddFragment(groupID, 100, 33, 0, 3, 0, make([]byte, 33))

	// 获取存在的组
	group, exists := reassembler.GetGroup(groupID)
	if !exists {
		t.Error("Group should exist")
	}
	if group == nil {
		t.Fatal("Group should not be nil")
	}
}

func TestFragmentReassembler_RemoveGroup(t *testing.T) {
	reassembler := NewFragmentReassembler()

	groupID := "test-group"

	// 添加分片创建组
	reassembler.AddFragment(groupID, 100, 33, 0, 3, 0, make([]byte, 33))

	// 验证组存在
	_, exists := reassembler.GetGroup(groupID)
	if !exists {
		t.Fatal("Group should exist")
	}

	// 移除组
	reassembler.RemoveGroup(groupID)

	// 验证组不存在
	_, exists = reassembler.GetGroup(groupID)
	if exists {
		t.Error("Group should not exist after removal")
	}
}

func TestCalculateFragments(t *testing.T) {
	tests := []struct {
		name           string
		dataSize       int
		expectedSize   int
		expectedCount  int
	}{
		{"small data, no fragment", 1000, 1000, 1},
		{"medium data, fragment", 20 * 1024, MaxFragmentSize, 2},
		{"large data, multiple fragments", 50 * 1024, MaxFragmentSize, 5},
		{"exact threshold", FragmentThreshold, FragmentThreshold, 1},
		{"just over threshold", FragmentThreshold + 1, MaxFragmentSize, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fragmentSize, totalFragments := CalculateFragments(tt.dataSize)
			if fragmentSize != tt.expectedSize {
				t.Errorf("FragmentSize: expected %d, got %d", tt.expectedSize, fragmentSize)
			}
			if totalFragments != tt.expectedCount {
				t.Errorf("TotalFragments: expected %d, got %d", tt.expectedCount, totalFragments)
			}
		})
	}
}

func TestGetFragmentData(t *testing.T) {
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}

	fragmentSize := 33
	totalFragments := 3

	// 获取第一个分片
	fragment0 := GetFragmentData(data, 0, fragmentSize, totalFragments)
	if len(fragment0) != fragmentSize {
		t.Errorf("Fragment 0 size: expected %d, got %d", fragmentSize, len(fragment0))
	}

	// 获取第二个分片
	fragment1 := GetFragmentData(data, 1, fragmentSize, totalFragments)
	if len(fragment1) != fragmentSize {
		t.Errorf("Fragment 1 size: expected %d, got %d", fragmentSize, len(fragment1))
	}

	// 获取最后一个分片（可能小于 fragmentSize）
	fragment2 := GetFragmentData(data, 2, fragmentSize, totalFragments)
	expectedSize := len(data) - 2*fragmentSize
	if len(fragment2) != expectedSize {
		t.Errorf("Fragment 2 size: expected %d, got %d", expectedSize, len(fragment2))
	}

	// 验证数据正确性
	allFragments := append(append(fragment0, fragment1...), fragment2...)
	if len(allFragments) != len(data) {
		t.Errorf("Total size: expected %d, got %d", len(data), len(allFragments))
	}
	for i := range data {
		if allFragments[i] != data[i] {
			t.Errorf("Data mismatch at index %d", i)
			break
		}
	}
}

func TestFragmentReassembler_ConcurrentAccess(t *testing.T) {
	reassembler := NewFragmentReassembler()

	groupID := "test-group"
	originalSize := 100
	fragmentSize := 33
	totalFragments := 3

	// 并发添加分片
	done := make(chan bool, totalFragments)
	for i := 0; i < totalFragments; i++ {
		go func(index int) {
			_, err := reassembler.AddFragment(groupID, originalSize, fragmentSize, index, totalFragments, 0, make([]byte, fragmentSize))
			if err != nil {
				t.Errorf("AddFragment failed for index %d: %v", index, err)
			}
			done <- true
		}(i)
	}

	// 等待所有分片添加完成
	for i := 0; i < totalFragments; i++ {
		<-done
	}

	// 验证组完整
	group, exists := reassembler.GetGroup(groupID)
	if !exists {
		t.Fatal("Group should exist")
	}
	if !group.IsComplete() {
		t.Error("Group should be complete after adding all fragments")
	}
}

func TestFragmentReassembler_MaxGroups(t *testing.T) {
	reassembler := NewFragmentReassembler()

	// 创建最大数量的组
	for i := 0; i < MaxFragmentGroups; i++ {
		groupID := fmt.Sprintf("group-%d", i)
		_, err := reassembler.AddFragment(groupID, 100, 33, 0, 3, int64(i), make([]byte, 33))
		if err != nil {
			t.Fatalf("AddFragment failed for group %d: %v", i, err)
		}
	}

	// 尝试添加超出限制的组（应该失败或触发清理）
	_, err := reassembler.AddFragment("overflow-group", 100, 33, 0, 3, int64(MaxFragmentGroups), make([]byte, 33))
	// 注意：如果清理了过期组，可能会成功；否则应该失败
	// 这里只验证不会 panic
	if err != nil {
		t.Logf("Expected error when exceeding max groups: %v", err)
	}
}

func TestFragmentReassembler_ExpiredCleanup(t *testing.T) {
	// 创建一个测试用的重组器，使用较短的超时时间
	reassembler := &FragmentReassembler{
		groups: make(map[string]*FragmentGroup),
	}

	// 创建过期组
	oldGroup := &FragmentGroup{
		GroupID:        "old-group",
		OriginalSize:   100,
		TotalFragments: 3,
		Fragments:      make([]*Fragment, 3),
		CreatedTime:    time.Now().Add(-FragmentGroupTimeout - time.Second),
	}
	reassembler.groups["old-group"] = oldGroup

	// 创建新组
	newGroup := &FragmentGroup{
		GroupID:        "new-group",
		OriginalSize:   100,
		TotalFragments: 3,
		Fragments:      make([]*Fragment, 3),
		CreatedTime:    time.Now(),
	}
	reassembler.groups["new-group"] = newGroup

	// 清理过期组
	reassembler.cleanupExpiredLocked()

	// 验证过期组被删除
	_, exists := reassembler.groups["old-group"]
	if exists {
		t.Error("Old group should be removed")
	}

	// 验证新组仍然存在
	_, exists = reassembler.groups["new-group"]
	if !exists {
		t.Error("New group should still exist")
	}
}

