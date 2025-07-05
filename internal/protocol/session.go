package protocol

import (
	"io"
	"sync"
	"tunnox-core/internal/cloud"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

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
