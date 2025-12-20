package constants

// 网络缓冲区大小常量
const (
	KB = 1024
	// MB 和 GB 已在 constants.go 中定义

	// CopyBufferSize 数据复制缓冲区大小（32KB）
	// 经过性能测试，32KB 在性能和内存使用之间取得最佳平衡
	CopyBufferSize = 32 * KB

	// TCPSocketBufferSize TCP socket 读写缓冲区大小（512KB）
	// 这是系统级缓冲区，由内核管理
	TCPSocketBufferSize = 512 * KB

	// WebSocketBufferSize WebSocket 读写缓冲区大小（64KB）
	WebSocketBufferSize = 64 * KB

	// SocksBufferSize SOCKS 代理缓冲区大小（32KB）
	SocksBufferSize = 32 * KB
)

// 上下文检查间隔常量
const (
	// ContextCheckInterval 上下文检查间隔（每 N 次迭代检查一次）
	ContextCheckInterval = 10000

	// BatchUpdateThreshold 批量更新阈值（字节）
	BatchUpdateThreshold = 1 * MB
)
