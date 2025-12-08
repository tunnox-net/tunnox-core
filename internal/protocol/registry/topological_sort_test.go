package registry

import (
	"testing"
)

func TestTopologicalSort_Simple(t *testing.T) {
	// graph[node] 表示 node 依赖的节点列表
	// 如果 b 依赖 a，则 graph["b"] = ["a"]
	// 初始化顺序应该是：先初始化 a，再初始化 b
	graph := map[string][]string{
		"a": {},      // a 没有依赖
		"b": {"a"},   // b 依赖 a
		"c": {"b"},   // c 依赖 b
	}

	result, err := TopologicalSort(graph)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// 验证顺序：a 应该在 b 之前，b 应该在 c 之前
	aIndex := indexOf(result, "a")
	bIndex := indexOf(result, "b")
	cIndex := indexOf(result, "c")

	if aIndex == -1 || bIndex == -1 || cIndex == -1 {
		t.Fatalf("Expected all nodes in result, got %v", result)
	}

	if aIndex >= bIndex {
		t.Fatalf("Expected 'a' before 'b', got order %v (a at %d, b at %d)", result, aIndex, bIndex)
	}
	if bIndex >= cIndex {
		t.Fatalf("Expected 'b' before 'c', got order %v (b at %d, c at %d)", result, bIndex, cIndex)
	}
}

func TestTopologicalSort_CircularDependency(t *testing.T) {
	graph := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"}, // 循环依赖
	}

	_, err := TopologicalSort(graph)
	if err == nil {
		t.Fatal("Expected error for circular dependency")
	}
}

func TestTopologicalSort_NoDependencies(t *testing.T) {
	graph := map[string][]string{
		"a": {},
		"b": {},
		"c": {},
	}

	result, err := TopologicalSort(graph)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("Expected 3 protocols, got %d", len(result))
	}
}

func indexOf(slice []string, item string) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}

