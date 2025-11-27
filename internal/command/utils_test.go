package command

import (
	"io"
	"testing"
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

func (m *UtilsMockSession) SetEventBus(eventBus interface{}) error {
	return nil
}

func (m *UtilsMockSession) GetEventBus() interface{} {
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

	// 测试链式调用
	result := utils.
		TcpMapCreate().
		PutRequest(map[string]interface{}{"port": 8080}).
		Timeout(5000).
		WithAuthentication(true).
		WithUserID("user123")

	if result.commandType != packet.TcpMapCreate {
		t.Errorf("Expected command type %v, got %v", packet.TcpMapCreate, result.commandType)
	}

	if result.timeout != 5000 {
		t.Errorf("Expected timeout 5000, got %v", result.timeout)
	}

	if !result.isAuthenticated {
		t.Errorf("Expected isAuthenticated true, got %v", result.isAuthenticated)
	}

	if result.userID != "user123" {
		t.Errorf("Expected userID 'user123', got %v", result.userID)
	}
}
