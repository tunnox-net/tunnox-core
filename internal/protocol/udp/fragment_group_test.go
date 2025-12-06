package udp

import (
	"testing"
)

func TestFragmentGroup(t *testing.T) {
	key := FragmentGroupKey{
		SessionID: 12345,
		StreamID:  0,
		PacketSeq: 100,
	}

	frag0 := []byte("fragment 0 data")
	frag1 := []byte("fragment 1 data")
	frag2 := []byte("fragment 2 data")
	expectedSize := len(frag0) + len(frag1) + len(frag2)

	group := NewFragmentGroup(key, 3, expectedSize)

	// 添加分片（乱序）
	if err := group.AddFragment(2, frag2); err != nil {
		t.Fatalf("AddFragment failed: %v", err)
	}

	if err := group.AddFragment(0, frag0); err != nil {
		t.Fatalf("AddFragment failed: %v", err)
	}

	if err := group.AddFragment(1, frag1); err != nil {
		t.Fatalf("AddFragment failed: %v", err)
	}

	if !group.IsComplete() {
		t.Fatal("Fragment group should be complete")
	}

	result, err := group.Reassemble()
	if err != nil {
		t.Fatalf("Reassemble failed: %v", err)
	}

	expected := append(frag0, append(frag1, frag2...)...)
	if len(result) != len(expected) {
		t.Fatalf("Reassembled size mismatch: expected %d, got %d", len(expected), len(result))
	}

	for i := range expected {
		if result[i] != expected[i] {
			t.Fatalf("Reassembled data mismatch at index %d", i)
		}
	}
}

func TestFragmentGroupDuplicateFragment(t *testing.T) {
	key := FragmentGroupKey{
		SessionID: 12345,
		StreamID:  0,
		PacketSeq: 100,
	}

	group := NewFragmentGroup(key, 2, 200)

	frag0 := []byte("fragment 0")
	if err := group.AddFragment(0, frag0); err != nil {
		t.Fatalf("AddFragment failed: %v", err)
	}

	// 重复添加应该被忽略
	frag0Dup := []byte("duplicate")
	if err := group.AddFragment(0, frag0Dup); err != nil {
		t.Fatalf("AddFragment failed: %v", err)
	}

	// 验证原始数据没有被覆盖
	if len(group.Fragments[0]) != len(frag0) {
		t.Fatal("Duplicate fragment should be ignored")
	}
}

func TestFragmentGroupInvalidFragSeq(t *testing.T) {
	key := FragmentGroupKey{
		SessionID: 12345,
		StreamID:  0,
		PacketSeq: 100,
	}

	group := NewFragmentGroup(key, 2, 200)

	err := group.AddFragment(5, []byte("invalid"))
	if err == nil {
		t.Fatal("Expected error for invalid fragSeq")
	}
}

