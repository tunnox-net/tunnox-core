package session

import (
	"io"
	"sync/atomic"
)

// copyWithControl 带流量统计和限速的数据拷贝（极致性能优化版）
// 🚀 优化点:
// 1. 移除所有热路径日志
// 2. 使用 512KB 大缓冲区
// 3. 极低频率的 context 检查 (每 10000 次)
// 4. 批量更新流量统计
func (b *TunnelBridge) copyWithControl(dst io.Writer, src io.Reader, direction string, counter *atomic.Int64) int64 {
	// 🚀 性能优化: 使用 32KB 缓冲区（性价比最优）
	buf := make([]byte, 32*1024)
	var total int64
	var batchCounter int64 // 批量统计，减少原子操作

	// 🚀 性能优化: 极低频率的 Context 检查
	checkCounter := 0
	const checkInterval = 10000 // 每 10000 次循环检查一次

	for {
		// 极低频率检查 context
		checkCounter++
		if checkCounter >= checkInterval {
			checkCounter = 0
			select {
			case <-b.Ctx().Done():
				counter.Add(batchCounter) // 提交剩余统计
				return total
			default:
			}
		}

		// 从源端读取
		nr, err := src.Read(buf)
		if nr > 0 {
			// 应用限速（如果启用）- 大多数情况下 rateLimiter 为 nil
			if b.rateLimiter != nil {
				if waitErr := b.rateLimiter.WaitN(b.Ctx(), nr); waitErr != nil {
					break
				}
			}

			// 写入目标端
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				total += int64(nw)
				batchCounter += int64(nw)
				// 🚀 批量更新统计（每 1MB 更新一次）
				if batchCounter >= 1024*1024 {
					counter.Add(batchCounter)
					batchCounter = 0
				}
			}
			if ew != nil {
				break
			}
			if nr != nw {
				break
			}
		}
		if err != nil {
			// UDP 超时错误处理
			if netErr, ok := err.(interface {
				Timeout() bool
				Temporary() bool
			}); ok && netErr.Timeout() && netErr.Temporary() {
				continue
			}
			break
		}
	}

	// 提交剩余的统计
	if batchCounter > 0 {
		counter.Add(batchCounter)
	}
	return total
}

// dynamicSourceWriter 动态获取 sourceForwarder 的 Writer 包装器（使用接口抽象）
// 用于在 target->source 方向时，每次写入都使用最新的 sourceForwarder
type dynamicSourceWriter struct {
	bridge *TunnelBridge
}

func (w *dynamicSourceWriter) Write(p []byte) (n int, err error) {
	w.bridge.sourceConnMu.RLock()
	sourceForwarder := w.bridge.sourceForwarder
	w.bridge.sourceConnMu.RUnlock()

	if sourceForwarder == nil {
		return 0, io.ErrClosedPipe
	}
	return sourceForwarder.Write(p)
}
