package stream

// StreamProcessorAccessor 流处理器访问器接口
// 用于获取流处理器的元数据信息（如ClientID、ConnectionID等）
type StreamProcessorAccessor interface {
	// GetClientID 获取客户端ID
	GetClientID() int64

	// GetConnectionID 获取连接ID
	GetConnectionID() string

	// GetMappingID 获取映射ID（如果适用）
	GetMappingID() string
}
