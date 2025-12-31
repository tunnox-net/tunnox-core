package iocopy

import (
	"net"
	"runtime"

	"golang.org/x/net/ipv4"
)

// udpBatchWriter UDP 批量写入器
// 使用 sendmmsg 系统调用批量发送 UDP 包，减少系统调用开销
// Linux: 使用 sendmmsg，单次系统调用发送多个包
// macOS/Windows: 回退到逐个发送
type udpBatchWriter struct {
	conn     *net.UDPConn
	pktConn  *ipv4.PacketConn
	messages []ipv4.Message
	count    int
	isLinux  bool
}

// newUDPBatchWriter 创建批量写入器
func newUDPBatchWriter(conn *net.UDPConn, batchSize int) *udpBatchWriter {
	bw := &udpBatchWriter{
		conn:     conn,
		pktConn:  ipv4.NewPacketConn(conn),
		messages: make([]ipv4.Message, batchSize),
		count:    0,
		isLinux:  runtime.GOOS == "linux",
	}
	return bw
}

// add 添加一个数据包到批量缓冲（零拷贝，调用者保证数据有效性直到 flush）
// 返回是否成功添加（缓冲区满时返回 false）
func (bw *udpBatchWriter) add(data []byte) bool {
	if bw.count >= len(bw.messages) {
		return false // 缓冲区满
	}
	bw.messages[bw.count].Buffers = [][]byte{data}
	bw.messages[bw.count].N = len(data)
	bw.messages[bw.count].Addr = nil // 已连接的 UDP，不需要地址
	bw.count++
	return true
}

// flush 刷新所有待发送的数据包
// 返回发送的总字节数和错误
func (bw *udpBatchWriter) flush() (int64, error) {
	if bw.count == 0 {
		return 0, nil
	}

	var totalBytes int64
	var err error

	if bw.isLinux {
		// Linux: 使用 sendmmsg 批量发送
		// WriteBatch 返回成功发送的消息数量
		sentCount, writeErr := bw.pktConn.WriteBatch(bw.messages[:bw.count], 0)
		err = writeErr
		// 计算实际发送的字节数（基于成功发送的消息数）
		for i := 0; i < sentCount; i++ {
			totalBytes += int64(len(bw.messages[i].Buffers[0]))
		}
	} else {
		// macOS/Windows: 逐个发送（已连接的 UDP 使用 Write）
		for i := 0; i < bw.count; i++ {
			msg := &bw.messages[i]
			_, err = bw.conn.Write(msg.Buffers[0])
			if err != nil {
				break
			}
			totalBytes += int64(len(msg.Buffers[0]))
		}
	}

	// 重置计数
	bw.count = 0

	return totalBytes, err
}
