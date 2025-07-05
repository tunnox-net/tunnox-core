package protocol

import (
	"io"
	"tunnox-core/internal/utils"
)

// Adapter 协议适配器统一接口

type Adapter interface {
	ConnectTo(serverAddr string) error
	ListenFrom(serverAddr string) error
	Name() string
	GetReader() io.Reader
	GetWriter() io.Writer
	Close()
	SetAddr(addr string)
	GetAddr() string
}

type BaseAdapter struct {
	utils.Dispose
	name string
	addr string
}

func (b *BaseAdapter) GetAddr() string  { return b.addr }
func (b *BaseAdapter) Name() string     { return b.name }
func (b *BaseAdapter) Addr() string     { return b.addr }
func (b *BaseAdapter) SetName(n string) { b.name = n }
func (b *BaseAdapter) SetAddr(a string) { b.addr = a }
