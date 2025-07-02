package tests

import (
	"testing"
	"tunnox-core/internal/conn"
	"tunnox-core/internal/packet"
)

func TestCommandTypeConstants(t *testing.T) {
	// 测试所有命令类型常量
	expectedTypes := map[packet.CommandType]string{
		packet.TcpMap:     "TcpMap",
		packet.HttpMap:    "HttpMap",
		packet.SocksMap:   "SocksMap",
		packet.DataIn:     "DataIn",
		packet.Forward:    "Forward",
		packet.DataOut:    "DataOut",
		packet.Disconnect: "Disconnect",
	}

	for cmdType, expectedName := range expectedTypes {
		if cmdType == 0 {
			t.Errorf("CommandType %s should not be zero", expectedName)
		}
	}
}

func TestInitPacket(t *testing.T) {
	// 测试InitPacket结构体
	initPacket := packet.InitPacket{
		ConnType:  conn.ClientControl,
		ClientId:  "test-client-123",
		SecretKey: "secret-key-456",
	}

	// 验证字段值
	if initPacket.ConnType != conn.ClientControl {
		t.Errorf("Expected ConnType %v, got %v", conn.ClientControl, initPacket.ConnType)
	}

	if initPacket.ClientId != "test-client-123" {
		t.Errorf("Expected ClientId %s, got %s", "test-client-123", initPacket.ClientId)
	}

	if initPacket.SecretKey != "secret-key-456" {
		t.Errorf("Expected SecretKey %s, got %s", "secret-key-456", initPacket.SecretKey)
	}
}

func TestAcceptPacket(t *testing.T) {
	// 测试AcceptPacket结构体
	acceptPacket := packet.AcceptPacket{
		ConnType: conn.DataTransfer,
		ClientId: "test-client-789",
		Token:    "token-abc",
		AuthCode: "auth-code-def",
	}

	// 验证字段值
	if acceptPacket.ConnType != conn.DataTransfer {
		t.Errorf("Expected ConnType %v, got %v", conn.DataTransfer, acceptPacket.ConnType)
	}

	if acceptPacket.ClientId != "test-client-789" {
		t.Errorf("Expected ClientId %s, got %s", "test-client-789", acceptPacket.ClientId)
	}

	if acceptPacket.Token != "token-abc" {
		t.Errorf("Expected Token %s, got %s", "token-abc", acceptPacket.Token)
	}

	if acceptPacket.AuthCode != "auth-code-def" {
		t.Errorf("Expected AuthCode %s, got %s", "auth-code-def", acceptPacket.AuthCode)
	}
}

func TestCommandPacket(t *testing.T) {
	// 测试CommandPacket结构体
	commandPacket := packet.CommandPacket{
		CommandType: packet.TcpMap,
		Token:       "command-token-123",
		SenderId:    "sender-456",
		ReceiverId:  "receiver-789",
		CommandBody: "command-body-data",
	}

	// 验证字段值
	if commandPacket.CommandType != packet.TcpMap {
		t.Errorf("Expected CommandType %v, got %v", packet.TcpMap, commandPacket.CommandType)
	}

	if commandPacket.Token != "command-token-123" {
		t.Errorf("Expected Token %s, got %s", "command-token-123", commandPacket.Token)
	}

	if commandPacket.SenderId != "sender-456" {
		t.Errorf("Expected SenderId %s, got %s", "sender-456", commandPacket.SenderId)
	}

	if commandPacket.ReceiverId != "receiver-789" {
		t.Errorf("Expected ReceiverId %s, got %s", "receiver-789", commandPacket.ReceiverId)
	}

	if commandPacket.CommandBody != "command-body-data" {
		t.Errorf("Expected CommandBody %s, got %s", "command-body-data", commandPacket.CommandBody)
	}
}

func TestCommandPacketWithDifferentTypes(t *testing.T) {
	// 测试不同命令类型的CommandPacket
	testCases := []struct {
		name        string
		commandType packet.CommandType
	}{
		{"TcpMap", packet.TcpMap},
		{"HttpMap", packet.HttpMap},
		{"SocksMap", packet.SocksMap},
		{"DataIn", packet.DataIn},
		{"Forward", packet.Forward},
		{"DataOut", packet.DataOut},
		{"Disconnect", packet.Disconnect},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			commandPacket := packet.CommandPacket{
				CommandType: tc.commandType,
				Token:       "test-token",
				SenderId:    "test-sender",
				ReceiverId:  "test-receiver",
				CommandBody: "test-body",
			}

			if commandPacket.CommandType != tc.commandType {
				t.Errorf("Expected CommandType %v, got %v", tc.commandType, commandPacket.CommandType)
			}
		})
	}
}

func TestInitPacketWithEmptyFields(t *testing.T) {
	// 测试空字段的InitPacket
	initPacket := packet.InitPacket{
		ConnType:  conn.ServiceControl,
		ClientId:  "",
		SecretKey: "",
	}

	// 验证字段值
	if initPacket.ConnType != conn.ServiceControl {
		t.Errorf("Expected ConnType %v, got %v", conn.ServiceControl, initPacket.ConnType)
	}

	if initPacket.ClientId != "" {
		t.Errorf("Expected empty ClientId, got %s", initPacket.ClientId)
	}

	if initPacket.SecretKey != "" {
		t.Errorf("Expected empty SecretKey, got %s", initPacket.SecretKey)
	}
}

func TestAcceptPacketWithEmptyFields(t *testing.T) {
	// 测试空字段的AcceptPacket
	acceptPacket := packet.AcceptPacket{
		ConnType: conn.ServerControlReply,
		ClientId: "",
		Token:    "",
		AuthCode: "",
	}

	// 验证字段值
	if acceptPacket.ConnType != conn.ServerControlReply {
		t.Errorf("Expected ConnType %v, got %v", conn.ServerControlReply, acceptPacket.ConnType)
	}

	if acceptPacket.ClientId != "" {
		t.Errorf("Expected empty ClientId, got %s", acceptPacket.ClientId)
	}

	if acceptPacket.Token != "" {
		t.Errorf("Expected empty Token, got %s", acceptPacket.Token)
	}

	if acceptPacket.AuthCode != "" {
		t.Errorf("Expected empty AuthCode, got %s", acceptPacket.AuthCode)
	}
}

func TestCommandPacketWithEmptyFields(t *testing.T) {
	// 测试空字段的CommandPacket
	commandPacket := packet.CommandPacket{
		CommandType: packet.DataIn,
		Token:       "",
		SenderId:    "",
		ReceiverId:  "",
		CommandBody: "",
	}

	// 验证字段值
	if commandPacket.CommandType != packet.DataIn {
		t.Errorf("Expected CommandType %v, got %v", packet.DataIn, commandPacket.CommandType)
	}

	if commandPacket.Token != "" {
		t.Errorf("Expected empty Token, got %s", commandPacket.Token)
	}

	if commandPacket.SenderId != "" {
		t.Errorf("Expected empty SenderId, got %s", commandPacket.SenderId)
	}

	if commandPacket.ReceiverId != "" {
		t.Errorf("Expected empty ReceiverId, got %s", commandPacket.ReceiverId)
	}

	if commandPacket.CommandBody != "" {
		t.Errorf("Expected empty CommandBody, got %s", commandPacket.CommandBody)
	}
}

func TestCommandTypeValues(t *testing.T) {
	// 测试命令类型的数值
	expectedValues := map[packet.CommandType]byte{
		packet.TcpMap:     2,
		packet.HttpMap:    3,
		packet.SocksMap:   4,
		packet.DataIn:     5,
		packet.Forward:    6,
		packet.DataOut:    7,
		packet.Disconnect: 8,
	}

	for cmdType, expectedValue := range expectedValues {
		if byte(cmdType) != expectedValue {
			t.Errorf("Expected CommandType %v to have value %d, got %d", cmdType, expectedValue, byte(cmdType))
		}
	}
}

func TestPacketStructSizes(t *testing.T) {
	// 测试结构体大小（可选，用于性能优化）
	initPacket := packet.InitPacket{}
	acceptPacket := packet.AcceptPacket{}
	commandPacket := packet.CommandPacket{}

	// 验证结构体不为空
	if initPacket.ConnType != 0 {
		t.Error("InitPacket should have zero value initially")
	}

	if acceptPacket.ConnType != 0 {
		t.Error("AcceptPacket should have zero value initially")
	}

	if commandPacket.CommandType != 0 {
		t.Error("CommandPacket should have zero value initially")
	}
}

func TestCommandTypeComparison(t *testing.T) {
	// 测试命令类型比较
	if packet.TcpMap >= packet.HttpMap {
		t.Error("TcpMap should be less than HttpMap")
	}

	if packet.HttpMap >= packet.SocksMap {
		t.Error("HttpMap should be less than SocksMap")
	}

	if packet.SocksMap >= packet.DataIn {
		t.Error("SocksMap should be less than DataIn")
	}

	if packet.DataIn >= packet.Forward {
		t.Error("DataIn should be less than Forward")
	}

	if packet.Forward >= packet.DataOut {
		t.Error("Forward should be less than DataOut")
	}

	if packet.DataOut >= packet.Disconnect {
		t.Error("DataOut should be less than Disconnect")
	}
}
