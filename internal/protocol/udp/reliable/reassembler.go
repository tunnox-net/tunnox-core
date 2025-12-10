package reliable

import (
	"io"
	"sync"

	coreErrors "tunnox-core/internal/core/errors"
)

// Reassembler 数据重组器
// 负责将分片的 UDP 数据包重组成完整的数据流
// 使用 io.Pipe 实现流式传输，自动处理背压
type Reassembler struct {
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	closed     bool
	mu         sync.Mutex
}

// NewReassembler 创建数据重组器
func NewReassembler() *Reassembler {
	pr, pw := io.Pipe()
	return &Reassembler{
		pipeReader: pr,
		pipeWriter: pw,
	}
}

// Write 写入分片数据
// 数据会自动合并到流中，应用层可以按需读取
func (r *Reassembler) Write(data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return coreErrors.New(coreErrors.ErrorTypePermanent, "reassembler closed")
	}

	_, err := r.pipeWriter.Write(data)
	if err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to write to pipe")
	}
	return nil
}

// Read 读取重组后的数据
// 实现 io.Reader 接口，支持流式读取
func (r *Reassembler) Read(p []byte) (int, error) {
	return r.pipeReader.Read(p)
}

// Close 关闭重组器
// 关闭后，Read 会返回 EOF
func (r *Reassembler) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true

	// 关闭写入端，触发 EOF
	if err := r.pipeWriter.Close(); err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to close pipe writer")
	}

	// 关闭读取端
	if err := r.pipeReader.Close(); err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to close pipe reader")
	}

	return nil
}
