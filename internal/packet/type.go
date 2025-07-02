package packet

type Type byte

const (
	JsonCommand Type = 1
	Compressed  Type = 2
	Encrypted   Type = 4
	Heartbeat   Type = 8
)
