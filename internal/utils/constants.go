package utils

// 网络相关常量
const (
	// DefaultChunkSize 默认分块大小，用于限速和读写操作
	DefaultChunkSize = 1024

	// DefaultBurstRatio 默认突发比例，用于限速器配置
	DefaultBurstRatio = 2

	// MinBurstSize 最小突发大小
	MinBurstSize = 1024
)

// 包相关常量
const (
	// PacketTypeSize 包类型字节大小
	PacketTypeSize = 1

	// PacketBodySizeBytes 包体大小字段字节数
	PacketBodySizeBytes = 4
)
