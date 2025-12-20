package session

// TunnelBridgeAccessor 隧道桥接访问器接口（用于API层和跨包访问）
// 避免循环依赖，提供最小化的访问接口
type TunnelBridgeAccessor interface {
	// GetTunnelID 获取隧道ID
	GetTunnelID() string

	// GetSourceConnectionID 获取源连接ID
	GetSourceConnectionID() string

	// GetTargetConnectionID 获取目标连接ID
	GetTargetConnectionID() string

	// GetMappingID 获取映射ID
	GetMappingID() string

	// GetClientID 获取客户端ID
	GetClientID() int64

	// IsActive 检查桥接是否活跃
	IsActive() bool

	// Close 关闭桥接
	Close() error
}
