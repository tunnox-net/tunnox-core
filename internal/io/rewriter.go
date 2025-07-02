package io

import (
	"context"
	"io"
	"sync"
	"tunnox-core/internal/utils"
)

type Stream struct {
	reader    io.Reader
	writer    io.Writer
	transLock sync.Mutex
	utils.Dispose
}

func NewStream(reader io.Reader, writer io.Writer, parentCtx context.Context) *Stream {
	stream := &Stream{reader: reader, writer: writer}
	stream.SetCtx(parentCtx, nil)
	return stream
}
