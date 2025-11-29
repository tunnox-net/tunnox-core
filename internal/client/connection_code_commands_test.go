package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestUpdateTrafficStatsFromMappings 测试流量统计更新
func TestUpdateTrafficStatsFromMappings(t *testing.T) {
	client := &TunnoxClient{
		localTrafficStats: make(map[string]*localMappingStats),
	}

	mappings := []MappingInfoCmd{
		{
			MappingID:     "mapping-1",
			BytesSent:     1000,
			BytesReceived: 2000,
		},
		{
			MappingID:     "mapping-2",
			BytesSent:     3000,
			BytesReceived: 4000,
		},
	}

	client.updateTrafficStatsFromMappings(mappings)

	// 验证 mapping-1
	stats1, exists1 := client.localTrafficStats["mapping-1"]
	assert.True(t, exists1)
	assert.Equal(t, int64(1000), stats1.bytesSent)
	assert.Equal(t, int64(2000), stats1.bytesReceived)

	// 验证 mapping-2
	stats2, exists2 := client.localTrafficStats["mapping-2"]
	assert.True(t, exists2)
	assert.Equal(t, int64(3000), stats2.bytesSent)
	assert.Equal(t, int64(4000), stats2.bytesReceived)
}

// TestParseListenAddress 测试监听地址解析
func TestParseListenAddress(t *testing.T) {
	tests := []struct {
		addr     string
		expected string
		port     int
		err      bool
	}{
		{"127.0.0.1:8888", "127.0.0.1", 8888, false},
		{"localhost:9999", "localhost", 9999, false},
		{"[::1]:8080", "::1", 8080, false},
		{"", "", 0, true},
		{"invalid", "", 0, true},
		{"127.0.0.1:70000", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			host, port, err := parseListenAddress(tt.addr)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, host)
				assert.Equal(t, tt.port, port)
			}
		})
	}
}

// TestParseTargetAddress 测试目标地址解析
func TestParseTargetAddress(t *testing.T) {
	tests := []struct {
		addr     string
		expected string
		port     int
		protocol string
		err      bool
	}{
		{"tcp://192.168.1.10:8080", "192.168.1.10", 8080, "tcp", false},
		{"udp://10.0.0.1:53", "10.0.0.1", 53, "udp", false},
		{"192.168.1.10:8080", "192.168.1.10", 8080, "tcp", false},
		{"", "", 0, "", true},
		{"invalid", "", 0, "", true},
		{"tcp://192.168.1.10:70000", "", 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			host, port, protocol, err := parseTargetAddress(tt.addr)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, host)
				assert.Equal(t, tt.port, port)
				assert.Equal(t, tt.protocol, protocol)
			}
		})
	}
}
