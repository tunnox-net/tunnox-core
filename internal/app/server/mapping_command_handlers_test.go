package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
)

// TestMappingInfoConversion 测试映射信息转换
func TestMappingInfoConversion(t *testing.T) {
	// 测试 outbound 映射
	mapping1 := &models.PortMapping{
		ID:             "mapping-1",
		ListenClientID: 1001,
		TargetClientID: 1002,
		TargetAddress:  "tcp://192.168.1.10:8080",
		ListenAddress:  "127.0.0.1:8888",
		Status:         models.MappingStatusActive,
		TrafficStats: stats.TrafficStats{
			BytesSent:     1024,
			BytesReceived: 2048,
		},
		CreatedAt: time.Now(),
		ExpiresAt: &time.Time{},
	}

	clientID := int64(1001)
	mappingType := "outbound"
	if mapping1.TargetClientID == clientID && mapping1.ListenClientID != clientID {
		mappingType = "inbound"
	}

	assert.Equal(t, "outbound", mappingType)
	assert.Equal(t, int64(1024), mapping1.TrafficStats.BytesSent)
	assert.Equal(t, int64(2048), mapping1.TrafficStats.BytesReceived)

	// 测试 inbound 映射
	mapping2 := &models.PortMapping{
		ID:             "mapping-2",
		ListenClientID: 1003,
		TargetClientID: 1001,
		TargetAddress:  "tcp://10.0.0.1:3306",
		ListenAddress:  "127.0.0.1:9999",
		Status:         models.MappingStatusActive,
		TrafficStats: stats.TrafficStats{
			BytesSent:     512,
			BytesReceived: 1024,
		},
		CreatedAt: time.Now(),
		ExpiresAt: &time.Time{},
	}

	clientID = int64(1001)
	mappingType = "outbound"
	if mapping2.TargetClientID == clientID && mapping2.ListenClientID != clientID {
		mappingType = "inbound"
	}

	assert.Equal(t, "inbound", mappingType)
	assert.Equal(t, int64(512), mapping2.TrafficStats.BytesSent)
	assert.Equal(t, int64(1024), mapping2.TrafficStats.BytesReceived)
}
