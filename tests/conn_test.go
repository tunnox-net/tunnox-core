package tests

import (
	"testing"
	"tunnox-core/internal/conn"
)

func TestConnTypeConstants(t *testing.T) {
	// 测试所有连接类型常量
	expectedTypes := map[conn.Type]string{
		conn.ClientControl:      "ClientControl",
		conn.ServerControlReply: "ServerControlReply",
		conn.DataTransfer:       "DataTransfer",
		conn.DataTransferReply:  "DataTransferReply",
	}

	for connType, expectedName := range expectedTypes {
		if connType == 0 {
			t.Errorf("ConnType %s should not be zero", expectedName)
		}
	}
}

func TestConnTypeString(t *testing.T) {
	// 测试连接类型的字符串表示
	testCases := []struct {
		name     string
		connType conn.Type
		expected string
	}{
		{"ClientControl", conn.ClientControl, "ClientControl"},
		{"ServerControlReply", conn.ServerControlReply, "ServerControlReply"},
		{"DataTransfer", conn.DataTransfer, "DataTransfer"},
		{"DataTransferReply", conn.DataTransferReply, "DataTransferReply"},
		{"Unknown", conn.Type(99), "Unknown(99)"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.connType.String()
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestConnTypeIsControl(t *testing.T) {
	// 测试控制类连接判断
	testCases := []struct {
		name     string
		connType conn.Type
		expected bool
	}{
		{"ClientControl", conn.ClientControl, true},
		{"ServerControlReply", conn.ServerControlReply, true},
		{"DataTransfer", conn.DataTransfer, false},
		{"DataTransferReply", conn.DataTransferReply, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.connType.IsControl()
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestConnTypeIsData(t *testing.T) {
	// 测试数据类连接判断
	testCases := []struct {
		name     string
		connType conn.Type
		expected bool
	}{
		{"ClientControl", conn.ClientControl, false},
		{"ServerControlReply", conn.ServerControlReply, false},
		{"DataTransfer", conn.DataTransfer, true},
		{"DataTransferReply", conn.DataTransferReply, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.connType.IsData()
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestConnTypeIsReply(t *testing.T) {
	// 测试回复/转发类连接判断
	testCases := []struct {
		name     string
		connType conn.Type
		expected bool
	}{
		{"ClientControl", conn.ClientControl, false},
		{"ServerControlReply", conn.ServerControlReply, true},
		{"DataTransfer", conn.DataTransfer, false},
		{"DataTransferReply", conn.DataTransferReply, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.connType.IsReply()
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestConnTypeValues(t *testing.T) {
	// 测试连接类型的数值
	expectedValues := map[conn.Type]byte{
		conn.ClientControl:      1,
		conn.ServerControlReply: 2,
		conn.DataTransfer:       3,
		conn.DataTransferReply:  4,
	}

	for connType, expectedValue := range expectedValues {
		if byte(connType) != expectedValue {
			t.Errorf("Expected ConnType %v to have value %d, got %d", connType, expectedValue, byte(connType))
		}
	}
}

func TestConnInfo(t *testing.T) {
	// 测试连接信息结构体
	connInfo := conn.Info{
		Type:       conn.ClientControl,
		ConnID:     "conn-123",
		NodeID:     "node-456",
		SourceID:   "source-789",
		TargetID:   "target-abc",
		PairConnID: "pair-def",
	}

	// 验证字段值
	if connInfo.Type != conn.ClientControl {
		t.Errorf("Expected Type %v, got %v", conn.ClientControl, connInfo.Type)
	}

	if connInfo.ConnID != "conn-123" {
		t.Errorf("Expected ConnID %s, got %s", "conn-123", connInfo.ConnID)
	}

	if connInfo.NodeID != "node-456" {
		t.Errorf("Expected NodeID %s, got %s", "node-456", connInfo.NodeID)
	}

	if connInfo.SourceID != "source-789" {
		t.Errorf("Expected SourceID %s, got %s", "source-789", connInfo.SourceID)
	}

	if connInfo.TargetID != "target-abc" {
		t.Errorf("Expected TargetID %s, got %s", "target-abc", connInfo.TargetID)
	}

	if connInfo.PairConnID != "pair-def" {
		t.Errorf("Expected PairConnID %s, got %s", "pair-def", connInfo.PairConnID)
	}
}

func TestConnInfoString(t *testing.T) {
	// 测试连接信息的字符串表示
	connInfo := conn.Info{
		Type:       conn.DataTransfer,
		ConnID:     "test-conn",
		NodeID:     "test-node",
		SourceID:   "test-source",
		TargetID:   "test-target",
		PairConnID: "test-pair",
	}

	result := connInfo.String()
	expected := "Connection{Type:DataTransfer, ConnID:test-conn, NodeID:test-node, SourceID:test-source, TargetID:test-target, PairConnID:test-pair}"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestConnInfoIsControl(t *testing.T) {
	// 测试控制类连接判断
	testCases := []struct {
		name     string
		connType conn.Type
		expected bool
	}{
		{"ClientControl", conn.ClientControl, true},
		{"ServerControlReply", conn.ServerControlReply, true},
		{"DataTransfer", conn.DataTransfer, false},
		{"DataTransferReply", conn.DataTransferReply, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			connInfo := conn.Info{Type: tc.connType}
			result := connInfo.IsControl()
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestConnInfoIsData(t *testing.T) {
	// 测试数据类连接判断
	testCases := []struct {
		name     string
		connType conn.Type
		expected bool
	}{
		{"ClientControl", conn.ClientControl, false},
		{"ServerControlReply", conn.ServerControlReply, false},
		{"DataTransfer", conn.DataTransfer, true},
		{"DataTransferReply", conn.DataTransferReply, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			connInfo := conn.Info{Type: tc.connType}
			result := connInfo.IsData()
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestConnInfoIsReply(t *testing.T) {
	// 测试回复/转发类连接判断
	testCases := []struct {
		name     string
		connType conn.Type
		expected bool
	}{
		{"ClientControl", conn.ClientControl, false},
		{"ServerControlReply", conn.ServerControlReply, true},
		{"DataTransfer", conn.DataTransfer, false},
		{"DataTransferReply", conn.DataTransferReply, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			connInfo := conn.Info{Type: tc.connType}
			result := connInfo.IsReply()
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestConnInfoHasPair(t *testing.T) {
	// 测试配对连接判断
	testCases := []struct {
		name       string
		pairConnID string
		expected   bool
	}{
		{"has_pair", "pair-123", true},
		{"no_pair", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			connInfo := conn.Info{PairConnID: tc.pairConnID}
			result := connInfo.HasPair()
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestConnInfoSetPair(t *testing.T) {
	// 测试设置配对连接ID
	connInfo := conn.Info{}

	// 初始状态应该没有配对
	if connInfo.HasPair() {
		t.Error("Initial state should not have pair")
	}

	// 设置配对连接ID
	connInfo.SetPair("new-pair-123")

	// 验证配对已设置
	if !connInfo.HasPair() {
		t.Error("Should have pair after SetPair")
	}

	if connInfo.PairConnID != "new-pair-123" {
		t.Errorf("Expected PairConnID %s, got %s", "new-pair-123", connInfo.PairConnID)
	}
}

func TestConnInfoClearPair(t *testing.T) {
	// 测试清除配对连接ID
	connInfo := conn.Info{PairConnID: "existing-pair"}

	// 初始状态应该有配对
	if !connInfo.HasPair() {
		t.Error("Initial state should have pair")
	}

	// 清除配对连接ID
	connInfo.ClearPair()

	// 验证配对已清除
	if connInfo.HasPair() {
		t.Error("Should not have pair after ClearPair")
	}

	if connInfo.PairConnID != "" {
		t.Errorf("Expected empty PairConnID, got %s", connInfo.PairConnID)
	}
}

func TestConnInfoWithEmptyFields(t *testing.T) {
	// 测试空字段的连接信息
	connInfo := conn.Info{
		Type:       conn.ClientControl,
		ConnID:     "",
		NodeID:     "",
		SourceID:   "",
		TargetID:   "",
		PairConnID: "",
	}

	// 验证字段值
	if connInfo.Type != conn.ClientControl {
		t.Errorf("Expected Type %v, got %v", conn.ClientControl, connInfo.Type)
	}

	if connInfo.ConnID != "" {
		t.Errorf("Expected empty ConnID, got %s", connInfo.ConnID)
	}

	if connInfo.NodeID != "" {
		t.Errorf("Expected empty NodeID, got %s", connInfo.NodeID)
	}

	if connInfo.SourceID != "" {
		t.Errorf("Expected empty SourceID, got %s", connInfo.SourceID)
	}

	if connInfo.TargetID != "" {
		t.Errorf("Expected empty TargetID, got %s", connInfo.TargetID)
	}

	if connInfo.PairConnID != "" {
		t.Errorf("Expected empty PairConnID, got %s", connInfo.PairConnID)
	}

	// 验证方法调用
	if connInfo.IsControl() != true {
		t.Error("ClientControl should be control type")
	}

	if connInfo.IsData() != false {
		t.Error("ClientControl should not be data type")
	}

	if connInfo.IsReply() != false {
		t.Error("ClientControl should not be reply type")
	}

	if connInfo.HasPair() != false {
		t.Error("Empty PairConnID should not have pair")
	}
}

func TestConnTypeComparison(t *testing.T) {
	// 测试连接类型比较
	if conn.ClientControl >= conn.ServerControlReply {
		t.Error("ClientControl should be less than ServerControlReply")
	}

	if conn.ServerControlReply >= conn.DataTransfer {
		t.Error("ServerControlReply should be less than DataTransfer")
	}

	if conn.DataTransfer >= conn.DataTransferReply {
		t.Error("DataTransfer should be less than DataTransferReply")
	}
}

func TestConnInfoStringWithEmptyFields(t *testing.T) {
	// 测试空字段的字符串表示
	connInfo := conn.Info{
		Type:       conn.DataTransferReply,
		ConnID:     "",
		NodeID:     "",
		SourceID:   "",
		TargetID:   "",
		PairConnID: "",
	}

	result := connInfo.String()
	expected := "Connection{Type:DataTransferReply, ConnID:, NodeID:, SourceID:, TargetID:, PairConnID:}"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}
