package api

import (
	"io"

	"tunnox-core/internal/packet"
	httppoll "tunnox-core/internal/protocol/httppoll"
	"tunnox-core/internal/utils"
)

// httppollStreamAdapter HTTP 长轮询流适配器
// 用于将 ServerStreamProcessor 适配为 io.Reader/io.Writer，以便在 SessionManager 中注册
// StreamManager.CreateStream 会检测到它是 PackageStreamer 并直接使用
type httppollStreamAdapter struct {
	streamProcessor *httppoll.ServerStreamProcessor
}

// GetStreamProcessor 获取内部的 ServerStreamProcessor（用于调试和元数据访问）
// 返回 StreamProcessorAccessor 接口，提供访问流处理器元数据的能力
func (a *httppollStreamAdapter) GetStreamProcessor() interface {
	GetClientID() int64
	GetConnectionID() string
	GetMappingID() string
} {
	return a.streamProcessor
}

func (a *httppollStreamAdapter) Read(p []byte) (int, error) {
	// HTTP 长轮询是无状态的，不通过 Read 读取数据
	// 数据通过 Push 请求和 Poll 响应处理
	return 0, io.EOF
}

func (a *httppollStreamAdapter) Write(p []byte) (int, error) {
	// HTTP 长轮询是无状态的，不通过 Write 写入数据
	// 数据通过 Push 请求和 Poll 响应处理
	return len(p), nil
}

func (a *httppollStreamAdapter) GetConnectionID() string {
	return a.streamProcessor.GetConnectionID()
}

// 实现 stream.PackageStreamer 接口，委托给 ServerStreamProcessor
func (a *httppollStreamAdapter) ReadPacket() (*packet.TransferPacket, int, error) {
	return a.streamProcessor.ReadPacket()
}

func (a *httppollStreamAdapter) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
	utils.Infof("httppollStreamAdapter: WritePacket called, delegating to ServerStreamProcessor, connID=%s", a.streamProcessor.GetConnectionID())
	return a.streamProcessor.WritePacket(pkt, useCompression, rateLimitBytesPerSecond)
}

func (a *httppollStreamAdapter) GetReader() io.Reader {
	return a.streamProcessor.GetReader()
}

func (a *httppollStreamAdapter) GetWriter() io.Writer {
	return a.streamProcessor.GetWriter()
}

func (a *httppollStreamAdapter) ReadExact(length int) ([]byte, error) {
	return a.streamProcessor.ReadExact(length)
}

func (a *httppollStreamAdapter) ReadAvailable(maxLength int) ([]byte, error) {
	return a.streamProcessor.ReadAvailable(maxLength)
}

func (a *httppollStreamAdapter) WriteExact(data []byte) error {
	return a.streamProcessor.WriteExact(data)
}

func (a *httppollStreamAdapter) Close() {
	a.streamProcessor.Close()
}

