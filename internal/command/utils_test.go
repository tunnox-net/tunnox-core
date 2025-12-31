package command

import (
	"io"
	"testing"
	"time"
	"tunnox-core/internal/core/events"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
)

// UtilsMockSession 模拟会话对象
type UtilsMockSession struct{}

func (m *UtilsMockSession) AcceptConnection(reader io.Reader, writer io.Writer) (*types.StreamConnection, error) {
	return nil, nil
}

func (m *UtilsMockSession) GetActiveChannels() int {
	return 0
}

func (m *UtilsMockSession) HandlePacket(connPacket *types.StreamPacket) error {
	return nil
}

func (m *UtilsMockSession) CloseConnection(connectionId string) error {
	return nil
}

func (m *UtilsMockSession) GetStreamManager() *stream.StreamManager {
	return nil
}

func (m *UtilsMockSession) GetStreamConnectionInfo(connectionId string) (*types.StreamConnection, bool) {
	return nil, false
}

func (m *UtilsMockSession) GetActiveConnections() int {
	return 0
}

func (m *UtilsMockSession) CreateConnection(reader io.Reader, writer io.Writer) (*types.Connection, error) {
	return nil, nil
}

func (m *UtilsMockSession) ProcessPacket(connID string, packet *packet.TransferPacket) error {
	return nil
}

func (m *UtilsMockSession) GetConnection(connID string) (*types.Connection, bool) {
	return nil, false
}

func (m *UtilsMockSession) ListConnections() []*types.Connection {
	return nil
}

func (m *UtilsMockSession) UpdateConnectionState(connID string, state types.ConnectionState) error {
	return nil
}

func (m *UtilsMockSession) SetEventBus(eventBus events.EventBus) error {
	return nil
}

func (m *UtilsMockSession) GetEventBus() events.EventBus {
	return nil
}

// ==================== Command集成相关方法 ====================

func (m *UtilsMockSession) RegisterCommandHandler(cmdType packet.CommandType, handler types.CommandHandler) error {
	return nil
}

func (m *UtilsMockSession) UnregisterCommandHandler(cmdType packet.CommandType) error {
	return nil
}

func (m *UtilsMockSession) ProcessCommand(connID string, cmd *packet.CommandPacket) (*types.CommandResponse, error) {
	return &types.CommandResponse{Success: true}, nil
}

func (m *UtilsMockSession) GetCommandRegistry() types.CommandRegistry {
	return nil
}

func (m *UtilsMockSession) GetCommandExecutor() types.CommandExecutor {
	return nil
}

func (m *UtilsMockSession) SetCommandExecutor(executor types.CommandExecutor) error {
	return nil
}

func TestCommandUtils_NewCommands(t *testing.T) {
	session := &UtilsMockSession{}
	utils := NewCommandUtils(session)

	// 测试连接管理类命令
	t.Run("Connection Commands", func(t *testing.T) {
		if utils.Connect().commandType != packet.Connect {
			t.Errorf("Connect() should set command type to %v, got %v", packet.Connect, utils.Connect().commandType)
		}

		if utils.Reconnect().commandType != packet.Reconnect {
			t.Errorf("Reconnect() should set command type to %v, got %v", packet.Reconnect, utils.Reconnect().commandType)
		}

		if utils.Disconnect().commandType != packet.Disconnect {
			t.Errorf("Disconnect() should set command type to %v, got %v", packet.Disconnect, utils.Disconnect().commandType)
		}

		if utils.Heartbeat().commandType != packet.HeartbeatCmd {
			t.Errorf("Heartbeat() should set command type to %v, got %v", packet.HeartbeatCmd, utils.Heartbeat().commandType)
		}
	})

	// 测试TCP映射类命令
	t.Run("TCP Mapping Commands", func(t *testing.T) {
		if utils.TcpMapCreate().commandType != packet.TcpMapCreate {
			t.Errorf("TcpMapCreate() should set command type to %v, got %v", packet.TcpMapCreate, utils.TcpMapCreate().commandType)
		}

		if utils.TcpMapDelete().commandType != packet.TcpMapDelete {
			t.Errorf("TcpMapDelete() should set command type to %v, got %v", packet.TcpMapDelete, utils.TcpMapDelete().commandType)
		}

		if utils.TcpMapUpdate().commandType != packet.TcpMapUpdate {
			t.Errorf("TcpMapUpdate() should set command type to %v, got %v", packet.TcpMapUpdate, utils.TcpMapUpdate().commandType)
		}

		if utils.TcpMapList().commandType != packet.TcpMapList {
			t.Errorf("TcpMapList() should set command type to %v, got %v", packet.TcpMapList, utils.TcpMapList().commandType)
		}

		if utils.TcpMapStatus().commandType != packet.TcpMapStatus {
			t.Errorf("TcpMapStatus() should set command type to %v, got %v", packet.TcpMapStatus, utils.TcpMapStatus().commandType)
		}
	})

	// 测试HTTP映射类命令
	t.Run("HTTP Mapping Commands", func(t *testing.T) {
		if utils.HttpMapCreate().commandType != packet.HttpMapCreate {
			t.Errorf("HttpMapCreate() should set command type to %v, got %v", packet.HttpMapCreate, utils.HttpMapCreate().commandType)
		}

		if utils.HttpMapDelete().commandType != packet.HttpMapDelete {
			t.Errorf("HttpMapDelete() should set command type to %v, got %v", packet.HttpMapDelete, utils.HttpMapDelete().commandType)
		}

		if utils.HttpMapUpdate().commandType != packet.HttpMapUpdate {
			t.Errorf("HttpMapUpdate() should set command type to %v, got %v", packet.HttpMapUpdate, utils.HttpMapUpdate().commandType)
		}

		if utils.HttpMapList().commandType != packet.HttpMapList {
			t.Errorf("HttpMapList() should set command type to %v, got %v", packet.HttpMapList, utils.HttpMapList().commandType)
		}

		if utils.HttpMapStatus().commandType != packet.HttpMapStatus {
			t.Errorf("HttpMapStatus() should set command type to %v, got %v", packet.HttpMapStatus, utils.HttpMapStatus().commandType)
		}
	})

	// 测试SOCKS映射类命令
	t.Run("SOCKS Mapping Commands", func(t *testing.T) {
		if utils.SocksMapCreate().commandType != packet.SocksMapCreate {
			t.Errorf("SocksMapCreate() should set command type to %v, got %v", packet.SocksMapCreate, utils.SocksMapCreate().commandType)
		}

		if utils.SocksMapDelete().commandType != packet.SocksMapDelete {
			t.Errorf("SocksMapDelete() should set command type to %v, got %v", packet.SocksMapDelete, utils.SocksMapDelete().commandType)
		}

		if utils.SocksMapUpdate().commandType != packet.SocksMapUpdate {
			t.Errorf("SocksMapUpdate() should set command type to %v, got %v", packet.SocksMapUpdate, utils.SocksMapUpdate().commandType)
		}

		if utils.SocksMapList().commandType != packet.SocksMapList {
			t.Errorf("SocksMapList() should set command type to %v, got %v", packet.SocksMapList, utils.SocksMapList().commandType)
		}

		if utils.SocksMapStatus().commandType != packet.SocksMapStatus {
			t.Errorf("SocksMapStatus() should set command type to %v, got %v", packet.SocksMapStatus, utils.SocksMapStatus().commandType)
		}
	})

	// 测试数据传输类命令
	t.Run("Data Transfer Commands", func(t *testing.T) {
		if utils.DataTransferStart().commandType != packet.DataTransferStart {
			t.Errorf("DataTransferStart() should set command type to %v, got %v", packet.DataTransferStart, utils.DataTransferStart().commandType)
		}

		if utils.DataTransferStop().commandType != packet.DataTransferStop {
			t.Errorf("DataTransferStop() should set command type to %v, got %v", packet.DataTransferStop, utils.DataTransferStop().commandType)
		}

		if utils.DataTransferStatus().commandType != packet.DataTransferStatus {
			t.Errorf("DataTransferStatus() should set command type to %v, got %v", packet.DataTransferStatus, utils.DataTransferStatus().commandType)
		}

		if utils.ProxyForward().commandType != packet.ProxyForward {
			t.Errorf("ProxyForward() should set command type to %v, got %v", packet.ProxyForward, utils.ProxyForward().commandType)
		}
	})

	// 测试系统管理类命令
	t.Run("System Management Commands", func(t *testing.T) {
		if utils.ConfigGet().commandType != packet.ConfigGet {
			t.Errorf("ConfigGet() should set command type to %v, got %v", packet.ConfigGet, utils.ConfigGet().commandType)
		}

		if utils.ConfigSet().commandType != packet.ConfigSet {
			t.Errorf("ConfigSet() should set command type to %v, got %v", packet.ConfigSet, utils.ConfigSet().commandType)
		}

		if utils.StatsGet().commandType != packet.StatsGet {
			t.Errorf("StatsGet() should set command type to %v, got %v", packet.StatsGet, utils.StatsGet().commandType)
		}

		if utils.LogGet().commandType != packet.LogGet {
			t.Errorf("LogGet() should set command type to %v, got %v", packet.LogGet, utils.LogGet().commandType)
		}

		if utils.HealthCheck().commandType != packet.HealthCheck {
			t.Errorf("HealthCheck() should set command type to %v, got %v", packet.HealthCheck, utils.HealthCheck().commandType)
		}
	})

	// 测试RPC类命令
	t.Run("RPC Commands", func(t *testing.T) {
		if utils.RpcInvoke().commandType != packet.RpcInvoke {
			t.Errorf("RpcInvoke() should set command type to %v, got %v", packet.RpcInvoke, utils.RpcInvoke().commandType)
		}

		if utils.RpcRegister().commandType != packet.RpcRegister {
			t.Errorf("RpcRegister() should set command type to %v, got %v", packet.RpcRegister, utils.RpcRegister().commandType)
		}

		if utils.RpcUnregister().commandType != packet.RpcUnregister {
			t.Errorf("RpcUnregister() should set command type to %v, got %v", packet.RpcUnregister, utils.RpcUnregister().commandType)
		}

		if utils.RpcList().commandType != packet.RpcList {
			t.Errorf("RpcList() should set command type to %v, got %v", packet.RpcList, utils.RpcList().commandType)
		}
	})

	// 测试兼容性命令
	t.Run("Compatibility Commands", func(t *testing.T) {
		if utils.TcpMap().commandType != packet.TcpMapCreate {
			t.Errorf("TcpMap() should set command type to %v, got %v", packet.TcpMapCreate, utils.TcpMap().commandType)
		}

		if utils.HttpMap().commandType != packet.HttpMapCreate {
			t.Errorf("HttpMap() should set command type to %v, got %v", packet.HttpMapCreate, utils.HttpMap().commandType)
		}

		if utils.SocksMap().commandType != packet.SocksMapCreate {
			t.Errorf("SocksMap() should set command type to %v, got %v", packet.SocksMapCreate, utils.SocksMap().commandType)
		}

		if utils.DataIn().commandType != packet.DataTransferStart {
			t.Errorf("DataIn() should set command type to %v, got %v", packet.DataTransferStart, utils.DataIn().commandType)
		}

		if utils.Forward().commandType != packet.ProxyForward {
			t.Errorf("Forward() should set command type to %v, got %v", packet.ProxyForward, utils.Forward().commandType)
		}

		if utils.DataOut().commandType != packet.DataTransferOut {
			t.Errorf("DataOut() should set command type to %v, got %v", packet.DataTransferOut, utils.DataOut().commandType)
		}
	})
}

func TestCommandUtils_Chaining(t *testing.T) {
	session := &UtilsMockSession{}
	utils := NewCommandUtils(session)

	// 测试链式调用（不使用 PutRequest，因为已移除，请使用 TypedCommandUtils）
	result := utils.
		TcpMapCreate().
		Timeout(5 * time.Second).
		WithAuthentication(true).
		WithUserID("user123")

	if result.commandType != packet.TcpMapCreate {
		t.Errorf("Expected command type %v, got %v", packet.TcpMapCreate, result.commandType)
	}

	if result.timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", result.timeout)
	}

	if !result.isAuthenticated {
		t.Errorf("Expected isAuthenticated true, got %v", result.isAuthenticated)
	}

	if result.userID != "user123" {
		t.Errorf("Expected userID 'user123', got %v", result.userID)
	}
}

// TestTypedCommandUtils_ChainingWithRequest 测试带请求数据的类型安全链式调用
func TestTypedCommandUtils_ChainingWithRequest(t *testing.T) {
	session := &UtilsMockSession{}

	// 定义类型安全的请求结构
	type TcpMapRequest struct {
		Port int `json:"port"`
	}
	type TcpMapResponse struct {
		Status string `json:"status"`
	}

	request := &TcpMapRequest{Port: 8080}
	response := &TcpMapResponse{}

	// 测试类型安全的链式调用
	result := NewTypedCommandUtils[TcpMapRequest, TcpMapResponse](session).
		WithCommand(packet.TcpMapCreate).
		PutRequest(request).
		ResultAs(response).
		Timeout(5 * time.Second).
		WithAuthentication(true).
		WithUserID("user123")

	if result.commandType != packet.TcpMapCreate {
		t.Errorf("Expected command type %v, got %v", packet.TcpMapCreate, result.commandType)
	}

	if result.timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", result.timeout)
	}

	if !result.isAuthenticated {
		t.Errorf("Expected isAuthenticated true, got %v", result.isAuthenticated)
	}

	if result.userID != "user123" {
		t.Errorf("Expected userID 'user123', got %v", result.userID)
	}

	// 验证请求数据被正确设置
	if result.requestData == nil || result.requestData.Port != 8080 {
		t.Errorf("Expected Port 8080, got %v", result.requestData)
	}
}

// ==================== TypedCommandUtils 测试 ====================

// 定义测试用的类型化请求和响应结构体
type TestTcpMapRequest struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type TestTcpMapResponse struct {
	MappingID string `json:"mapping_id"`
	Status    string `json:"status"`
}

func TestTypedCommandUtils_NewInstance(t *testing.T) {
	session := &UtilsMockSession{}
	utils := NewTypedCommandUtils[TestTcpMapRequest, TestTcpMapResponse](session)

	if utils == nil {
		t.Fatal("NewTypedCommandUtils should not return nil")
	}

	if utils.timeout != 30*time.Second {
		t.Errorf("Expected default timeout 30s, got %v", utils.timeout)
	}
}

func TestTypedCommandUtils_Chaining(t *testing.T) {
	session := &UtilsMockSession{}

	// 测试类型安全的链式调用
	request := &TestTcpMapRequest{Port: 8080, Protocol: "tcp"}
	response := &TestTcpMapResponse{}

	utils := NewTypedCommandUtils[TestTcpMapRequest, TestTcpMapResponse](session).
		WithCommand(packet.TcpMapCreate).
		PutRequest(request).
		ResultAs(response).
		Timeout(5 * time.Second).
		WithAuthentication(true).
		WithUserID("user123")

	if utils.commandType != packet.TcpMapCreate {
		t.Errorf("Expected command type %v, got %v", packet.TcpMapCreate, utils.commandType)
	}

	if utils.timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", utils.timeout)
	}

	if !utils.isAuthenticated {
		t.Errorf("Expected isAuthenticated true, got %v", utils.isAuthenticated)
	}

	if utils.userID != "user123" {
		t.Errorf("Expected userID 'user123', got %v", utils.userID)
	}

	// 验证请求数据被正确设置
	if utils.requestData == nil {
		t.Fatal("Expected requestData to be set")
	}

	if utils.requestData.Port != 8080 {
		t.Errorf("Expected Port 8080, got %d", utils.requestData.Port)
	}

	if utils.requestData.Protocol != "tcp" {
		t.Errorf("Expected Protocol 'tcp', got %s", utils.requestData.Protocol)
	}

	// 验证响应数据结构被正确设置
	if utils.responseData == nil {
		t.Fatal("Expected responseData to be set")
	}
}

func TestTypedCommandUtils_WithMethods(t *testing.T) {
	session := &UtilsMockSession{}

	t.Run("WithConnectionID", func(t *testing.T) {
		utils := NewTypedCommandUtils[TestTcpMapRequest, TestTcpMapResponse](session).
			WithConnectionID("conn-123")

		if utils.connectionID != "conn-123" {
			t.Errorf("Expected connectionID 'conn-123', got %v", utils.connectionID)
		}
	})

	t.Run("WithRequestID", func(t *testing.T) {
		utils := NewTypedCommandUtils[TestTcpMapRequest, TestTcpMapResponse](session).
			WithRequestID("req-456")

		if utils.requestID != "req-456" {
			t.Errorf("Expected requestID 'req-456', got %v", utils.requestID)
		}
	})

	t.Run("WithCommandId", func(t *testing.T) {
		utils := NewTypedCommandUtils[TestTcpMapRequest, TestTcpMapResponse](session).
			WithCommandId("cmd-789")

		if utils.commandId != "cmd-789" {
			t.Errorf("Expected commandId 'cmd-789', got %v", utils.commandId)
		}
	})

	t.Run("WithStartTime", func(t *testing.T) {
		startTime := time.Now()
		utils := NewTypedCommandUtils[TestTcpMapRequest, TestTcpMapResponse](session).
			WithStartTime(startTime)

		if utils.startTime != startTime {
			t.Errorf("Expected startTime %v, got %v", startTime, utils.startTime)
		}
	})

	t.Run("WithEndTime", func(t *testing.T) {
		endTime := time.Now()
		utils := NewTypedCommandUtils[TestTcpMapRequest, TestTcpMapResponse](session).
			WithEndTime(endTime)

		if utils.endTime != endTime {
			t.Errorf("Expected endTime %v, got %v", endTime, utils.endTime)
		}
	})
}

func TestTypedCommandUtils_GetResponse(t *testing.T) {
	session := &UtilsMockSession{}

	response := &TestTcpMapResponse{MappingID: "map-001", Status: "active"}

	utils := NewTypedCommandUtils[TestTcpMapRequest, TestTcpMapResponse](session).
		ResultAs(response)

	got := utils.GetResponse()
	if got == nil {
		t.Fatal("Expected GetResponse() to return non-nil")
	}

	if got.MappingID != "map-001" {
		t.Errorf("Expected MappingID 'map-001', got %s", got.MappingID)
	}

	if got.Status != "active" {
		t.Errorf("Expected Status 'active', got %s", got.Status)
	}
}

func TestTypedCommandUtils_ThrowOn(t *testing.T) {
	session := &UtilsMockSession{}

	customErrorHandled := false
	customErrorHandler := func(err error) error {
		customErrorHandled = true
		return err
	}

	utils := NewTypedCommandUtils[TestTcpMapRequest, TestTcpMapResponse](session).
		ThrowOn(customErrorHandler)

	// 验证错误处理器被设置
	if utils.errorHandler == nil {
		t.Fatal("Expected errorHandler to be set")
	}

	// 触发错误处理器
	_ = utils.errorHandler(nil)
	if !customErrorHandled {
		t.Error("Expected custom error handler to be called")
	}
}

// 测试不同类型参数的泛型
type SimpleRequest struct {
	ID string `json:"id"`
}

type SimpleResponse struct {
	OK bool `json:"ok"`
}

func TestTypedCommandUtils_DifferentTypes(t *testing.T) {
	session := &UtilsMockSession{}

	// 测试不同类型的请求和响应
	request := &SimpleRequest{ID: "test-id"}
	response := &SimpleResponse{}

	utils := NewTypedCommandUtils[SimpleRequest, SimpleResponse](session).
		WithCommand(packet.HealthCheck).
		PutRequest(request).
		ResultAs(response)

	if utils.requestData == nil {
		t.Fatal("Expected requestData to be set")
	}

	if utils.requestData.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got %s", utils.requestData.ID)
	}
}
