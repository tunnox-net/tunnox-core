package client

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/core/dispose"
	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/utils"
)

const (
	httppollDefaultPushTimeout = 20 * time.Second
	httppollDefaultPollTimeout = 20 * time.Second
	httppollMaxRetries         = 3
	httppollRetryInterval      = 1 * time.Second
	httppollMaxRequestSize     = 1024 * 1024 // 1MB
)

// HTTPLongPollingConn HTTP 长轮询连接
// 实现 net.Conn 接口，用于与 StreamProcessor 集成
type HTTPLongPollingConn struct {
	*dispose.ManagerBase

	baseURL      string
	connectionID string // ConnectionID（唯一标识，在创建时就确定，不会改变）
	clientID     int64
	token        string
	instanceID   string // 客户端实例标识（进程级别的唯一UUID）
	mappingID    string // 映射ID（隧道连接才有，指令通道为空）
	connType     string // 连接类型："control" | "data"

	// 上行连接（发送数据）
	pushURL    string
	pushClient *http.Client
	pushMu     sync.Mutex

	// 下行连接（接收数据）
	pollURL    string
	pollClient *http.Client

	// Base64 数据通道（接收 Base64 编码的数据）
	base64DataChan chan string

	// 读取缓冲区（字节流缓冲区，Base64 解码后的数据追加到这里）
	readBuffer []byte
	readBufMu  sync.Mutex

	// 用于保存 ReadPacket 读取的数据，以便在读取非目标包时恢复
	peekBuffer []byte
	peekBufMu  sync.Mutex

	// 写入缓冲区（缓冲多次 Write 调用，直到完整包）
	writeBuffer bytes.Buffer
	writeBufMu  sync.Mutex
	writeFlush  chan struct{} // 触发刷新缓冲区

	// 控制
	closed  bool
	closeMu sync.Mutex

	// 流模式（隧道建立后切换到流模式，直接转发原始数据，不再解析数据包格式）
	streamMode bool
	streamMu   sync.RWMutex

	// 分片重组器（用于接收端重组分片）
	fragmentReassembler *httppoll.FragmentReassembler

	// 地址信息（用于实现 net.Conn 接口）
	localAddr  net.Addr
	remoteAddr net.Addr
}

// UpdateClientID 更新客户端 ID（握手后调用）
func (c *HTTPLongPollingConn) UpdateClientID(newClientID int64) {
	c.pushMu.Lock()
	defer c.pushMu.Unlock()

	oldClientID := c.clientID
	c.clientID = newClientID
	corelog.Infof("HTTP long polling: updated clientID from %d to %d", oldClientID, newClientID)
}

// NewHTTPLongPollingConn 创建 HTTP 长轮询连接
func NewHTTPLongPollingConn(ctx context.Context, baseURL string, clientID int64, token string, instanceID string, mappingID string) (*HTTPLongPollingConn, error) {
	// 确保 baseURL 以 / 结尾
	baseURL = strings.TrimSuffix(baseURL, "/")
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}

	// 生成 ConnectionID（唯一标识，在创建时就确定，不会改变）
	connID, err := utils.GenerateUUID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate connection ID: %w", err)
	}
	connectionID := "conn_" + connID[:8] // 使用 "conn_" 前缀 + UUID 前8位

	// 确定连接类型
	connType := "control"
	if mappingID != "" {
		connType = "data"
	}

	conn := &HTTPLongPollingConn{
		ManagerBase:  dispose.NewManager("HTTPLongPollingConn", ctx),
		baseURL:      baseURL,
		connectionID: connectionID,
		clientID:     clientID,
		token:        token,
		instanceID:   instanceID,
		mappingID:    mappingID,
		connType:     connType,
		pushURL:      baseURL + "/tunnox/v1/push",
		pollURL:      baseURL + "/tunnox/v1/poll",
		pushClient: &http.Client{
			Timeout: httppollDefaultPushTimeout,
		},
		pollClient: &http.Client{
			Timeout: httppollDefaultPollTimeout + 5*time.Second, // 轮询超时 + 缓冲
		},
		base64DataChan:      make(chan string, 100),
		writeFlush:          make(chan struct{}, 1),
		fragmentReassembler: httppoll.NewFragmentReassembler(), // 创建分片重组器
		localAddr:           &httppollAddr{network: "httppoll", addr: "local"},
		remoteAddr:          &httppollAddr{network: "httppoll", addr: baseURL},
	}

	// 注册清理处理器
	conn.AddCleanHandler(conn.onClose)

	// 启动接收循环
	corelog.Debugf("HTTP long polling: starting pollLoop goroutine, clientID=%d, pollURL=%s", conn.clientID, conn.pollURL)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				corelog.Errorf("HTTP long polling: pollLoop panic: %v, stack: %s", r, string(debug.Stack()))
			}
		}()
		corelog.Debugf("HTTP long polling: pollLoop goroutine started, about to call pollLoop(), clientID=%d", conn.clientID)
		conn.pollLoop()
		corelog.Debugf("HTTP long polling: pollLoop goroutine finished, clientID=%d", conn.clientID)
	}()

	// 启动写入刷新循环（定期刷新缓冲区）
	go conn.writeFlushLoop()

	corelog.Infof("HTTP long polling: connection established to %s", baseURL)
	return conn, nil
}

// onClose 资源清理
func (c *HTTPLongPollingConn) onClose() error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return nil
	}
	c.closed = true
	c.closeMu.Unlock()

	// 关闭通道
	close(c.base64DataChan)

	return nil
}

// dialHTTPLongPolling 建立 HTTP 长轮询连接
func dialHTTPLongPolling(ctx context.Context, baseURL string, clientID int64, token string, instanceID string, mappingID string) (net.Conn, error) {
	return NewHTTPLongPollingConn(ctx, baseURL, clientID, token, instanceID, mappingID)
}
