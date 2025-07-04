package packet

type Type byte

const (
	JsonCommand Type = 1
	Compressed  Type = 2
	Encrypted   Type = 4
	Heartbeat   Type = 8
)

// IsHeartbeat 判断是否为心跳包
func (t Type) IsHeartbeat() bool {
	return t&Heartbeat != 0
}

// IsJsonCommand 判断是否为JsonCommand包
func (t Type) IsJsonCommand() bool {
	return t&JsonCommand != 0
}

// IsCompressed 判断是否压缩
func (t Type) IsCompressed() bool {
	return t&Compressed != 0
}

// IsEncrypted 判断是否加密
func (t Type) IsEncrypted() bool {
	return t&Encrypted != 0
}

// HasCompression 检查包是否包含压缩标志
func (t Type) HasCompression() bool {
	return t&Compressed != 0
}

// HasEncryption 检查包是否包含加密标志
func (t Type) HasEncryption() bool {
	return t&Encrypted != 0
}
