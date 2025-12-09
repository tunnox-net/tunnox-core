package registry

import (
	coreErrors "tunnox-core/internal/core/errors"
)

// TopologicalSort 拓扑排序（解析初始化顺序）
// graph[node] 表示 node 依赖的节点列表
// 返回按依赖顺序排序的协议名称列表（依赖的节点在前）
func TopologicalSort(graph map[string][]string) ([]string, error) {
	// 计算入度：每个节点依赖多少个其他节点
	inDegree := make(map[string]int)
	// 初始化所有节点的入度为0
	for node := range graph {
		inDegree[node] = 0
	}
	// 计算每个节点的入度：如果 b 依赖 a，则 b 的入度+1
	for node, deps := range graph {
		// 确保依赖的节点在 inDegree 中
		for _, dep := range deps {
			if _, exists := inDegree[dep]; !exists {
				inDegree[dep] = 0
			}
		}
		// node 的入度 = 它依赖的节点数量
		inDegree[node] = len(deps)
	}

	// 找到所有入度为 0 的节点（不依赖其他节点的节点）
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

		// 找到所有依赖当前节点的节点，减少它们的入度
		// 因为 node 已经被处理，依赖它的节点可以减少一个依赖
		for otherNode, deps := range graph {
			for _, dep := range deps {
				if dep == node {
					// otherNode 依赖 node，node 已处理，所以 otherNode 的入度减1
					inDegree[otherNode]--
					if inDegree[otherNode] == 0 {
						queue = append(queue, otherNode)
					}
				}
			}
		}
	}

	// 检查是否有循环依赖
	if len(result) != len(inDegree) {
		return nil, coreErrors.New(coreErrors.ErrorTypePermanent, "circular dependency detected in protocol initialization")
	}

	return result, nil
}

