package session

import (
	"io"
	"sync/atomic"

	"tunnox-core/internal/utils"
)

// copyWithControl 带流量统计和限速的数据拷贝
func (b *TunnelBridge) copyWithControl(dst io.Writer, src io.Reader, direction string, counter *atomic.Int64) int64 {
	buf := make([]byte, 32*1024) // 32KB buffer
	var total int64

	for {
		// 检查是否已取消
		select {
		case <-b.Ctx().Done():
			utils.Debugf("TunnelBridge[%s]: %s cancelled", b.tunnelID, direction)
			return total
		default:
		}

		// 从源端读取
		nr, err := src.Read(buf)
		if nr > 0 {
			// 应用限速（如果启用）
			if b.rateLimiter != nil {
				// 使用 bridge 的 context 进行限速等待
				if err := b.rateLimiter.WaitN(b.Ctx(), nr); err != nil {
					utils.Errorf("TunnelBridge[%s]: %s rate limit error: %v", b.tunnelID, direction, err)
					break
				}
			}

			// 写入目标端
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				total += int64(nw)
				counter.Add(int64(nw)) // 更新流量统计
			}
			if ew != nil {
				if ew != io.EOF {
					utils.Debugf("TunnelBridge[%s]: %s write error: %v", b.tunnelID, direction, ew)
				}
				break
			}
			if nr != nw {
				utils.Errorf("TunnelBridge[%s]: %s short write", b.tunnelID, direction)
				break
			}
		}
		if err != nil {
			// ✅ UDP 连接的超时错误是临时错误，不应该导致连接关闭
			if netErr, ok := err.(interface {
				Timeout() bool
				Temporary() bool
			}); ok && netErr.Timeout() && netErr.Temporary() {
				// UDP 超时错误，继续等待
				utils.Debugf("TunnelBridge[%s]: %s UDP timeout, continuing...", b.tunnelID, direction)
				continue
			}
			if err != io.EOF {
				utils.Debugf("TunnelBridge[%s]: %s read error: %v (total bytes: %d)", b.tunnelID, direction, err, total)
			}
			break
		}
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

