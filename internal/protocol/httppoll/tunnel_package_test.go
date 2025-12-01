package httppoll

import (
	"testing"
)

func TestEncodeDecodeTunnelPackage(t *testing.T) {
	pkg := &TunnelPackage{
		ConnectionID: "conn_7A51UQCb",
		ClientID:      12345,
		MappingID:     "pmap_xxx",
		TunnelType:   "data",
		Type:         "TunnelOpen",
		Data: map[string]interface{}{
			"tunnel_id": "tun_xxx",
			"mapping_id": "pmap_xxx",
		},
	}
	
	encoded, err := EncodeTunnelPackage(pkg)
	if err != nil {
		t.Fatalf("EncodeTunnelPackage failed: %v", err)
	}
	
	if encoded == "" {
		t.Fatal("encoded package is empty")
	}
	
	decoded, err := DecodeTunnelPackage(encoded)
	if err != nil {
		t.Fatalf("DecodeTunnelPackage failed: %v", err)
	}
	
	if decoded.ConnectionID != pkg.ConnectionID {
		t.Errorf("ConnectionID mismatch: expected %s, got %s", pkg.ConnectionID, decoded.ConnectionID)
	}
	if decoded.ClientID != pkg.ClientID {
		t.Errorf("ClientID mismatch: expected %d, got %d", pkg.ClientID, decoded.ClientID)
	}
	if decoded.MappingID != pkg.MappingID {
		t.Errorf("MappingID mismatch: expected %s, got %s", pkg.MappingID, decoded.MappingID)
	}
	if decoded.TunnelType != pkg.TunnelType {
		t.Errorf("TunnelType mismatch: expected %s, got %s", pkg.TunnelType, decoded.TunnelType)
	}
	if decoded.Type != pkg.Type {
		t.Errorf("Type mismatch: expected %s, got %s", pkg.Type, decoded.Type)
	}
}

func TestValidateConnectionID(t *testing.T) {
	tests := []struct {
		name    string
		connID  string
		want    bool
	}{
		{"valid", "conn_7A51UQCb", true},
		{"valid long", "conn_7A51UQCb1234567890abcdef", true},
		{"empty", "", false},
		{"too short", "conn_", false},
		{"no prefix", "7A51UQCb", false}, // 必须有 "conn_" 前缀
		{"too long", string(make([]byte, 200)), false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateConnectionID(tt.connID); got != tt.want {
				t.Errorf("ValidateConnectionID(%q) = %v, want %v", tt.connID, got, tt.want)
			}
		})
	}
}

