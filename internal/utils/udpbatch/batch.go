// Package udpbatch 提供 UDP 批量发送/接收功能
// 使用 sendmmsg/recvmmsg 系统调用减少系统调用开销
package udpbatch

import (
	"net"
	"runtime"
	"sync"

	"golang.org/x/net/ipv4"
)

const (
	// BatchSize 批量操作的消息数量
	// 32 是一个平衡点：太小减少不了系统调用，太大增加延迟
	BatchSize = 32

	// MaxPacketSize UDP 最大包大小
	MaxPacketSize = 65535
)

// BatchWriter UDP 批量写入器
// 使用 sendmmsg 批量发送 UDP 包，减少系统调用
type BatchWriter struct {
	conn     *net.UDPConn
	pktConn  *ipv4.PacketConn
	messages []ipv4.Message
	buffers  [][]byte
	count    int
	mu       sync.Mutex
	addr     net.Addr // 目标地址（用于已连接的 UDP）
}

// NewBatchWriter 创建批量写入器
func NewBatchWriter(conn *net.UDPConn) *BatchWriter {
	bw := &BatchWriter{
		conn:     conn,
		pktConn:  ipv4.NewPacketConn(conn),
		messages: make([]ipv4.Message, BatchSize),
		buffers:  make([][]byte, BatchSize),
		count:    0,
	}

	// 预分配缓冲区
	for i := 0; i < BatchSize; i++ {
		bw.buffers[i] = make([]byte, MaxPacketSize)
		bw.messages[i].Buffers = [][]byte{bw.buffers[i]}
	}

	return bw
}

// NewBatchWriterWithAddr 创建带目标地址的批量写入器
func NewBatchWriterWithAddr(conn *net.UDPConn, addr net.Addr) *BatchWriter {
	bw := NewBatchWriter(conn)
	bw.addr = addr
	return bw
}

// Add 添加一个数据包到批量缓冲
// 如果缓冲区满，自动刷新
// 返回是否需要调用者手动刷新（当数据被添加但未满时返回 false）
func (bw *BatchWriter) Add(data []byte, addr net.Addr) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	// 复制数据到预分配缓冲区
	n := copy(bw.buffers[bw.count], data)
	bw.messages[bw.count].Buffers[0] = bw.buffers[bw.count][:n]
	bw.messages[bw.count].N = n
	bw.messages[bw.count].Addr = addr
	bw.count++

	// 缓冲区满，自动刷新
	if bw.count >= BatchSize {
		return bw.flushLocked()
	}

	return nil
}

// AddDirect 直接添加数据（零拷贝，调用者保证数据有效性）
// data 必须在 Flush 之前保持有效
func (bw *BatchWriter) AddDirect(data []byte, addr net.Addr) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	bw.messages[bw.count].Buffers = [][]byte{data}
	bw.messages[bw.count].N = len(data)
	bw.messages[bw.count].Addr = addr
	bw.count++

	if bw.count >= BatchSize {
		return bw.flushLocked()
	}

	return nil
}

// Flush 刷新所有待发送的数据包
func (bw *BatchWriter) Flush() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.flushLocked()
}

// flushLocked 内部刷新（需要持有锁）
func (bw *BatchWriter) flushLocked() error {
	if bw.count == 0 {
		return nil
	}

	// 使用 WriteBatch 批量发送
	// Linux 上使用 sendmmsg，其他平台回退到逐个发送
	var err error
	if runtime.GOOS == "linux" {
		_, err = bw.pktConn.WriteBatch(bw.messages[:bw.count], 0)
	} else {
		// macOS/Windows 不支持 sendmmsg，逐个发送
		for i := 0; i < bw.count; i++ {
			msg := &bw.messages[i]
			if msg.Addr != nil {
				_, err = bw.conn.WriteToUDP(msg.Buffers[0], msg.Addr.(*net.UDPAddr))
			} else if bw.addr != nil {
				_, err = bw.conn.Write(msg.Buffers[0])
			}
			if err != nil {
				break
			}
		}
	}

	// 重置计数
	bw.count = 0

	// 恢复预分配缓冲区引用（AddDirect 可能修改了它）
	for i := 0; i < BatchSize; i++ {
		bw.messages[i].Buffers = [][]byte{bw.buffers[i]}
	}

	return err
}

// Count 返回当前缓冲的包数量
func (bw *BatchWriter) Count() int {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.count
}

// BatchReader UDP 批量读取器
// 使用 recvmmsg 批量接收 UDP 包
type BatchReader struct {
	conn     *net.UDPConn
	pktConn  *ipv4.PacketConn
	messages []ipv4.Message
	buffers  [][]byte
	results  []ReadResult
	readIdx  int
	readEnd  int
	mu       sync.Mutex
}

// ReadResult 单个包的读取结果
type ReadResult struct {
	Data []byte
	Addr net.Addr
	N    int
}

// NewBatchReader 创建批量读取器
func NewBatchReader(conn *net.UDPConn) *BatchReader {
	br := &BatchReader{
		conn:     conn,
		pktConn:  ipv4.NewPacketConn(conn),
		messages: make([]ipv4.Message, BatchSize),
		buffers:  make([][]byte, BatchSize),
		results:  make([]ReadResult, BatchSize),
		readIdx:  0,
		readEnd:  0,
	}

	// 预分配缓冲区
	for i := 0; i < BatchSize; i++ {
		br.buffers[i] = make([]byte, MaxPacketSize)
		br.messages[i].Buffers = [][]byte{br.buffers[i]}
	}

	return br
}

// Read 读取一个数据包
// 如果内部缓冲区为空，批量读取新数据
func (br *BatchReader) Read() (*ReadResult, error) {
	br.mu.Lock()
	defer br.mu.Unlock()

	// 如果还有缓冲的数据，直接返回
	if br.readIdx < br.readEnd {
		result := &br.results[br.readIdx]
		br.readIdx++
		return result, nil
	}

	// 批量读取
	var n int
	var err error

	if runtime.GOOS == "linux" {
		n, err = br.pktConn.ReadBatch(br.messages, 0)
	} else {
		// macOS/Windows 回退到单次读取
		nn, addr, readErr := br.conn.ReadFromUDP(br.buffers[0])
		if readErr != nil {
			return nil, readErr
		}
		br.results[0].Data = br.buffers[0][:nn]
		br.results[0].Addr = addr
		br.results[0].N = nn
		br.readIdx = 1
		br.readEnd = 1
		return &br.results[0], nil
	}

	if err != nil {
		return nil, err
	}

	if n == 0 {
		return nil, nil
	}

	// 转换结果
	for i := 0; i < n; i++ {
		msg := &br.messages[i]
		br.results[i].Data = br.buffers[i][:msg.N]
		br.results[i].Addr = msg.Addr
		br.results[i].N = msg.N
	}

	br.readIdx = 1
	br.readEnd = n

	return &br.results[0], nil
}

// ReadBatch 批量读取多个数据包
// 返回实际读取的数量
func (br *BatchReader) ReadBatch(results []ReadResult) (int, error) {
	br.mu.Lock()
	defer br.mu.Unlock()

	var n int
	var err error

	if runtime.GOOS == "linux" {
		n, err = br.pktConn.ReadBatch(br.messages[:len(results)], 0)
	} else {
		// macOS/Windows 回退到单次读取
		nn, addr, readErr := br.conn.ReadFromUDP(br.buffers[0])
		if readErr != nil {
			return 0, readErr
		}
		results[0].Data = br.buffers[0][:nn]
		results[0].Addr = addr
		results[0].N = nn
		return 1, nil
	}

	if err != nil {
		return 0, err
	}

	// 转换结果
	for i := 0; i < n; i++ {
		msg := &br.messages[i]
		results[i].Data = br.buffers[i][:msg.N]
		results[i].Addr = msg.Addr
		results[i].N = msg.N
	}

	return n, nil
}
