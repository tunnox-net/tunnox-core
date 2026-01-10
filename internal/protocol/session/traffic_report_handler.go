package session

import (
	"encoding/json"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// HandleTrafficReport 处理客户端上报的流量统计
func (s *SessionManager) HandleTrafficReport(connPacket *types.StreamPacket) error {
	if connPacket.Packet.CommandPacket == nil {
		return coreerrors.New(coreerrors.CodeInvalidPacket, "command packet is nil")
	}

	cmd := connPacket.Packet.CommandPacket

	// 解析流量上报请求
	var req packet.TrafficReportRequest
	if err := json.Unmarshal([]byte(cmd.CommandBody), &req); err != nil {
		corelog.Errorf("TrafficReportHandler: failed to parse request: %v", err)
		return coreerrors.Wrap(err, coreerrors.CodeInvalidPacket, "invalid traffic report request")
	}

	corelog.Infof("TrafficReportHandler: received report - MappingID=%s, Sent=%d, Recv=%d",
		req.MappingID, req.BytesSent, req.BytesReceived)

	// 验证映射是否存在
	if s.cloudControl == nil {
		corelog.Warn("TrafficReportHandler: cloud control not configured")
		return nil // 不返回错误，避免影响客户端
	}

	mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		corelog.Errorf("TrafficReportHandler: mapping not found %s: %v", req.MappingID, err)
		return nil // 不返回错误，映射可能已被删除
	}

	// 更新映射的流量统计
	trafficStats := mapping.TrafficStats
	trafficStats.BytesSent += req.BytesSent
	trafficStats.BytesReceived += req.BytesReceived
	trafficStats.LastUpdated = time.Now()

	// 保存到存储
	if err := s.cloudControl.UpdatePortMappingStats(req.MappingID, &trafficStats); err != nil {
		corelog.Errorf("TrafficReportHandler: failed to update traffic stats: %v", err)
		return nil // 不返回错误，避免影响客户端
	}

	corelog.Infof("TrafficReportHandler: updated stats for %s - TotalSent=%d, TotalRecv=%d",
		req.MappingID, trafficStats.BytesSent, trafficStats.BytesReceived)

	return nil
}
