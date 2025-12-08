package stream

// IStreamProcessorAccessor 流处理器访问器接口
// 用于获取流处理器的元数据信息（如ClientID、ConnectionID等）
// 遵循编码规范：接口使用 I 前缀，访问器接口使用 Accessor 后缀
type IStreamProcessorAccessor interface {
	// GetClientID 获取客户端ID
	GetClientID() int64

	// GetConnectionID 获取连接ID
	GetConnectionID() string

	// GetMappingID 获取映射ID（如果适用）
	GetMappingID() string
}

