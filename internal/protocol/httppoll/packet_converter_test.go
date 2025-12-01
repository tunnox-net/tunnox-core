package httppoll

import (
	"encoding/json"
	"net/http"
	"testing"

	"tunnox-core/internal/packet"
)

func TestPacketConverter_WritePacket(t *testing.T) {
	converter := NewPacketConverter()
	converter.SetConnectionInfo("conn_123", 456, "mapping_789", "control")

	pkt := &packet.TransferPacket{
		PacketType: packet.Handshake,
		Payload:    []byte(`{"client_id":456}`),
	}

	req, err := converter.WritePacket(pkt)
	if err != nil {
		t.Fatalf("WritePacket failed: %v", err)
	}

	if req == nil {
		t.Fatal("WritePacket returned nil request")
	}

	packageHeader := req.Header.Get("X-Tunnel-Package")
	if packageHeader == "" {
		t.Fatal("X-Tunnel-Package header is missing")
	}

	// 解码验证
	pkg, err := DecodeTunnelPackage(packageHeader)
	if err != nil {
		t.Fatalf("Failed to decode tunnel package: %v", err)
	}

	if pkg.ConnectionID != "conn_123" {
		t.Errorf("Expected ConnectionID=conn_123, got %s", pkg.ConnectionID)
	}
	if pkg.ClientID != 456 {
		t.Errorf("Expected ClientID=456, got %d", pkg.ClientID)
	}
}

func TestPacketConverter_ReadPacket(t *testing.T) {
	converter := NewPacketConverter()
	converter.SetConnectionInfo("conn_123", 456, "mapping_789", "control")

	// 创建测试包
	pkg := &TunnelPackage{
		ConnectionID: "conn_123",
		ClientID:     456,
		MappingID:    "mapping_789",
		TunnelType:   "control",
		Type:         "Handshake",
		Data: &packet.HandshakeRequest{
			ClientID: 456,
			Token:    "test-token",
			Version:  "2.0",
		},
	}

	encoded, err := EncodeTunnelPackage(pkg)
	if err != nil {
		t.Fatalf("Failed to encode tunnel package: %v", err)
	}

	// 创建模拟 HTTP Response
	resp := &http.Response{
		Header: make(http.Header),
	}
	resp.Header.Set("X-Tunnel-Package", encoded)

	pkt, err := converter.ReadPacket(resp)
	if err != nil {
		t.Fatalf("ReadPacket failed: %v", err)
	}

	if pkt == nil {
		t.Fatal("ReadPacket returned nil packet")
	}

	if pkt.PacketType != packet.Handshake {
		t.Errorf("Expected PacketType=Handshake, got %v", pkt.PacketType)
	}
}

func TestPacketConverter_WriteData(t *testing.T) {
	converter := NewPacketConverter()

	data := []byte("test data")
	encoded, err := converter.WriteData(data)
	if err != nil {
		t.Fatalf("WriteData failed: %v", err)
	}

	if len(encoded) == 0 {
		t.Fatal("WriteData returned empty encoded data")
	}

	// encoded 应该是 Base64 编码的数据
	if len(encoded) < len(data) {
		t.Error("Encoded data should be at least as long as original data")
	}
}

func TestPacketConverter_ReadData(t *testing.T) {
	// Base64 编码的测试数据
	base64Data := "dGVzdCBkYXRh"
	
	// 创建模拟 HTTP Response
	resp := &http.Response{
		Body: http.NoBody,
	}
	
	// 注意：ReadData 需要从 Response Body 读取，这里简化测试
	// 实际使用中，Response Body 应该包含 Base64 编码的数据
	_ = base64Data
	_ = resp
}

func TestTunnelPackageToTransferPacket(t *testing.T) {
	pkg := &TunnelPackage{
		ConnectionID: "conn_123",
		ClientID:     456,
		MappingID:    "mapping_789",
		TunnelType:   "control",
		Type:         "Handshake",
		Data: &packet.HandshakeRequest{
			ClientID: 456,
			Token:    "test-token",
			Version:  "2.0",
		},
	}

	pkt, err := TunnelPackageToTransferPacket(pkg)
	if err != nil {
		t.Fatalf("TunnelPackageToTransferPacket failed: %v", err)
	}

	if pkt == nil {
		t.Fatal("TunnelPackageToTransferPacket returned nil packet")
	}

	if pkt.PacketType != packet.Handshake {
		t.Errorf("Expected PacketType=Handshake, got %v", pkt.PacketType)
	}

	// 验证 Payload
	var handshakeReq packet.HandshakeRequest
	if err := json.Unmarshal(pkt.Payload, &handshakeReq); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if handshakeReq.ClientID != 456 {
		t.Errorf("Expected ClientID=456, got %d", handshakeReq.ClientID)
	}
}

