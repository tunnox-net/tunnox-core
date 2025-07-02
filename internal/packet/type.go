package packet

type Type byte

const (
	ConnInit   Type = 1
	ConnAccept Type = 2
	Command    Type = 3
)
