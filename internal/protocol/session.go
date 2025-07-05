package protocol

import (
	"io"
	"sync"
	"tunnox-core/internal/cloud"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// ConnectionSession用于统一处理业务逻辑
// 将所有的协议通过AcceptConnection做成conn / ClientID 映射，并关联PackageStreamer，
// 后需就都只基于并关联PackageStreamer 操作流

type ConnectionSession struct {
	cloudApi cloud.CloudControlAPI
	connMap  map[io.Reader]string
	streamer map[string]stream.PackageStreamer

	connMapLock  sync.RWMutex
	streamerLock sync.RWMutex

	utils.Dispose
}

func (s *ConnectionSession) AcceptConnection(reader io.Reader, writer io.Writer) {
	ps := stream.NewPackageStream(reader, writer, s.Ctx())
	ps.AddCloseFunc(func() {
		s.connMapLock.Lock()
		defer s.connMapLock.Unlock()
		//delete(s.connMap, conn)
	})
	//ps.ReadPacket()
}
