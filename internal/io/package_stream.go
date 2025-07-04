package io

import (
	"context"
	"io"
	"sync"
	"tunnox-core/internal/utils"
)

type PackageStream struct {
	reader    io.Reader
	writer    io.Writer
	transLock sync.Mutex
	utils.Dispose
}

func NewPackageStream(reader io.Reader, writer io.Writer, parentCtx context.Context) *PackageStream {
	stream := &PackageStream{reader: reader, writer: writer}
	stream.SetCtx(parentCtx, nil)
	return stream
}
