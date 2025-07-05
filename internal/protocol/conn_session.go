package protocol

import (
	"context"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

type ConnectionSession struct {
	utils.Dispose
	ps  stream.PackageStreamer
	ctx context.Context
}

func NewConnectionSession(ps stream.PackageStreamer, parentCtx context.Context) *ConnectionSession {
	s := &ConnectionSession{
		ps:  ps,
		ctx: parentCtx,
	}
	s.SetCtx(parentCtx, s.onClose)
	return s
}

func (s *ConnectionSession) Run() {
	for {
		pkt, _, err := s.ps.ReadPacket()
		if err != nil {
			return
		}
		s.handlePacket(pkt)
	}
}

func (s *ConnectionSession) handlePacket(pkt *packet.TransferPacket) {
	if pkt.PacketType.IsHeartbeat() {

	}
	if pkt.PacketType.IsJsonCommand() {

	}
}

func (s *ConnectionSession) onClose() {
	s.ps.Close()
}
