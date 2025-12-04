package session

import (
	"bytes"
	"context"
	"net"
	"strconv"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
)

const (
	httppollServerDefaultTimeout = 30 * time.Second
	httppollServerMaxTimeout     = 60 * time.Second
	httppollServerChannelSize    = 100
	packetTypeSize               = 1
	packetBodySizeBytes          = 4
)

// ServerHTTPLongPollingConn 服务器端 HTTP 长轮询连接
// 实现 net.Conn 接口，将 HTTP 请求/响应转换为双向流
type ServerHTTPLongPollingConn struct {
	*dispose.ManagerBase

	clientID  int64
	mappingID string // 映射ID（隧道连接才有，指令通道为空）

	// Base64 数据通道（接收 Base64 编码的数据，来自 HTTP POST）
	base64PushDataChan chan string

	// 下行数据（服务器 → 客户端）：优先级队列（解决心跳包干扰问题）
	pollDataQueue *PriorityQueue
	pollDataChan  chan []byte // 用于 PollData 的阻塞 channel（单元素 channel，用于阻塞等待）
	pollSeq       uint64
	pollMu        sync.Mutex
	pollWaitChan  chan struct{} // 用于通知 PollData 有数据可用（非阻塞信号）

	// 读取缓冲区（处理部分读取）
	readBuffer []byte
	readBufMu  sync.Mutex

	// 写入缓冲区（缓冲多次 Write 调用，直到完整包）
	writeBuffer bytes.Buffer
	writeBufMu  sync.Mutex
	writeFlush  chan struct{} // 触发刷新缓冲区

	// ConnectionID（唯一标识，在创建时就确定，不会改变）
	connectionID string
	connectionMu sync.RWMutex

	// 控制
	closed  bool
	closeMu sync.RWMutex

	// 流模式标志（隧道建立后切换到流模式，不再解析数据包格式）
	streamMode bool
	streamMu   sync.RWMutex

	// 地址信息（用于实现 net.Conn 接口）
	localAddr  net.Addr
	remoteAddr net.Addr
}

// NewServerHTTPLongPollingConn 创建服务器端 HTTP 长轮询连接
func NewServerHTTPLongPollingConn(ctx context.Context, clientID int64) *ServerHTTPLongPollingConn {
	conn := &ServerHTTPLongPollingConn{
		ManagerBase:        dispose.NewManager("ServerHTTPLongPollingConn", ctx),
		clientID:           clientID,
		base64PushDataChan: make(chan string, httppollServerChannelSize),
		pollDataQueue:      NewPriorityQueue(3),    // 最多缓存3个心跳包
		pollDataChan:       make(chan []byte, 1),   // 单元素 channel，用于阻塞等待
		pollWaitChan:       make(chan struct{}, 1), // 非阻塞信号，通知 PollData 有数据可用
		writeFlush:         make(chan struct{}, 1),
		localAddr:          &httppollServerAddr{network: "httppoll", addr: "server"},
		remoteAddr:         &httppollServerAddr{network: "httppoll", addr: strconv.FormatInt(clientID, 10)},
	}

	conn.AddCleanHandler(conn.onClose)

	// 启动写入刷新循环
	go conn.writeFlushLoop()

	// 启动优先级队列调度循环
	go conn.pollDataScheduler()

	return conn
}

// onClose 资源清理
func (c *ServerHTTPLongPollingConn) onClose() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	close(c.base64PushDataChan)
	close(c.pollDataChan)

	return nil
}
