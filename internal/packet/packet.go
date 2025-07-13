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

type CommandType byte

const (
	TcpMap     CommandType = 2 //添加Tcp映射端口 Server->Client
	HttpMap    CommandType = 3 //添加Http映射 Server->Client
	SocksMap   CommandType = 4 //添加Socks映射 Server->Client
	DataIn     CommandType = 5 //客户端Tcp监听端口收到新的连接, 通知服务端准备透传 Client->Server
	Forward    CommandType = 6 //服务端检测到需要经其他的服务端中转，通知其他的服务端准备透传 Server->Server
	DataOut    CommandType = 7 //服务端通知目标客户端，准备开始透传 Server -> Client
	Disconnect CommandType = 8 //连接断开，可以任何方向
	RpcInvoke  CommandType = 9 //与连接无关的RPC命令，供系统扩展使用，会提供二级Key
)

// InitPacket 初始化数据包
type InitPacket struct {
	Version   string `json:"version"`
	ClientID  string `json:"client_id"`
	AuthCode  string `json:"auth_code"`
	SecretKey string `json:"secret_key"`
	NodeID    string `json:"node_id"`
	IPAddress string `json:"ip_address"`
	Type      string `json:"type"`
}

// AcceptPacket 接受数据包
type AcceptPacket struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	ClientID  string `json:"client_id"`
	Token     string `json:"token,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

type TransferPacket struct {
	PacketType    Type
	CommandPacket *CommandPacket
}

type CommandPacket struct {
	CommandType CommandType
	CommandId   string // 客户端生成的唯一命令ID
	Token       string
	SenderId    string
	ReceiverId  string
	CommandBody string
}
