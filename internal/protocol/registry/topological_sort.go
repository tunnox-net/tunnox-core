package registry

import (
	coreErrors "tunnox-core/internal/core/errors"
)

// TopologicalSort 拓扑排序（解析初始化顺序）
// 返回按依赖顺序排序的协议名称列表
func TopologicalSort(graph map[string][]string) ([]string, error) {
	// 计算入度
	inDegree := make(map[string]int)
	for node := range graph {
		inDegree[node] = 0
	}
	for _, deps := range graph {
		for _, dep := range deps {
			inDegree[dep]++
		}
	}

	// 找到所有入度为 0 的节点
	queue := make([]string, 0)
	for node, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, node)
		}
	}

	result := make([]string, 0)
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		// 减少依赖节点的入度
		for _, dep := range graph[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	// 检查是否有循环依赖
	if len(result) != len(graph) {
		return nil, coreErrors.New(coreErrors.ErrorTypePermanent, "circular dependency detected in protocol initialization")
	}

	return result, nil
}

