package protocol

import (
	"context"
	"tunnox-core/internal/utils"
)

// ProtocolAdapter 协议适配器统一接口
// Start: 启动监听/服务
// Close: 关闭并释放资源
// Dispose: 支持树型资源管理
// Name: 返回协议适配器名称
// Addr: 返回监听地址

type ProtocolAdapter interface {
	Start(ctx context.Context) error
	Close() error
	IsClosed() bool
	SetCtx(parent context.Context, onClose func())
	Ctx() context.Context
	Name() string
	Addr() string
}

// BaseAdapter 提供Dispose树型管理的基础实现
// 其它协议适配器可匿名嵌入

type BaseAdapter struct {
	utils.Dispose
	name string
	addr string
}

func (b *BaseAdapter) Name() string     { return b.name }
func (b *BaseAdapter) Addr() string     { return b.addr }
func (b *BaseAdapter) SetName(n string) { b.name = n }
func (b *BaseAdapter) SetAddr(a string) { b.addr = a }
