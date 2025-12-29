package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseListenAddress(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		wantHost    string
		wantPort    int
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid IPv4 with port",
			addr:     "0.0.0.0:7788",
			wantHost: "0.0.0.0",
			wantPort: 7788,
			wantErr:  false,
		},
		{
			name:     "valid localhost",
			addr:     "127.0.0.1:9999",
			wantHost: "127.0.0.1",
			wantPort: 9999,
			wantErr:  false,
		},
		{
			name:     "valid IPv6",
			addr:     "[::1]:8080",
			wantHost: "::1",
			wantPort: 8080,
			wantErr:  false,
		},
		{
			name:        "missing port",
			addr:        "0.0.0.0",
			wantErr:     true,
			errContains: "invalid listen address format",
		},
		{
			name:        "invalid port",
			addr:        "0.0.0.0:abc",
			wantErr:     true,
			errContains: "invalid port",
		},
		{
			name:        "port out of range",
			addr:        "0.0.0.0:65536",
			wantErr:     true,
			errContains: "out of range",
		},
		{
			name:        "empty address",
			addr:        "",
			wantErr:     true,
			errContains: "listen address is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := ParseListenAddress(tt.addr)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantPort, port)
		})
	}
}

func TestParseTargetAddress(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		wantHost    string
		wantPort    int
		wantProto   string
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid URL format with protocol",
			addr:      "tcp://10.51.22.69:3306",
			wantHost:  "10.51.22.69",
			wantPort:  3306,
			wantProto: "tcp",
			wantErr:   false,
		},
		{
			name:      "valid URL format UDP",
			addr:      "udp://192.168.1.1:53",
			wantHost:  "192.168.1.1",
			wantPort:  53,
			wantProto: "udp",
			wantErr:   false,
		},
		{
			name:      "valid host:port format (defaults to tcp)",
			addr:      "10.51.22.69:3306",
			wantHost:  "10.51.22.69",
			wantPort:  3306,
			wantProto: "tcp",
			wantErr:   false,
		},
		{
			name:      "valid localhost",
			addr:      "tcp://localhost:8080",
			wantHost:  "localhost",
			wantPort:  8080,
			wantProto: "tcp",
			wantErr:   false,
		},
		{
			name:      "socks5 proxy dynamic target (port 0)",
			addr:      "socks5://0.0.0.0:0",
			wantHost:  "0.0.0.0",
			wantPort:  0,
			wantProto: "socks5",
			wantErr:   false,
		},
		{
			name:      "socks5 with valid port",
			addr:      "socks5://127.0.0.1:1080",
			wantHost:  "127.0.0.1",
			wantPort:  1080,
			wantProto: "socks5",
			wantErr:   false,
		},
		{
			name:        "missing port",
			addr:        "tcp://10.51.22.69",
			wantErr:     true,
			errContains: "missing port",
		},
		{
			name:        "invalid port",
			addr:        "tcp://10.51.22.69:abc",
			wantErr:     true,
			errContains: "invalid target address format",
		},
		{
			name:        "port out of range",
			addr:        "tcp://10.51.22.69:65536",
			wantErr:     true,
			errContains: "out of range",
		},
		{
			name:        "empty address",
			addr:        "",
			wantErr:     true,
			errContains: "target address is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, proto, err := ParseTargetAddress(tt.addr)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantPort, port)
			assert.Equal(t, tt.wantProto, proto)
		})
	}
}
