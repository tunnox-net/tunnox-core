# tunnel_bridge.go 拆分计划

**原文件**: 838行  
**拆分策略**: 分步骤进行，每步完成后测试验证

---

## 拆分步骤

### 第一步：提取接口定义和包装器（最独立，风险最小）
**目标文件**: `tunnel_bridge_interfaces.go`
**内容**:
- DataForwarder 接口
- StreamDataForwarder 接口
- checkStreamDataForwarder 函数
- streamDataForwarderWrapper 结构体和方法
- streamDataForwarderAdapter 结构体和方法
- createDataForwarder 函数

**预计行数**: ~280行  
**风险**: ⭐ (最低)  
**原因**: 这些是独立的接口和工具函数，不影响核心逻辑

---

### 第二步：提取流量统计相关方法（相对独立）
**目标文件**: `tunnel_bridge_stats.go`
**内容**:
- periodicTrafficReport 方法
- reportTrafficStats 方法

**预计行数**: ~50行  
**风险**: ⭐⭐ (低)  
**原因**: 流量统计是独立功能，不影响数据转发

---

### 第三步：提取Getter方法（简单，风险小）
**目标文件**: `tunnel_bridge_accessors.go`
**内容**:
- GetTunnelID
- GetSourceConnectionID
- GetTargetConnectionID
- GetMappingID
- GetClientID
- IsActive

**预计行数**: ~80行  
**风险**: ⭐ (最低)  
**原因**: 只是简单的getter方法，不涉及复杂逻辑

---

### 第四步：提取连接设置方法（需要仔细）
**目标文件**: `tunnel_bridge_connection.go`
**内容**:
- SetTargetConnection
- SetTargetConnectionLegacy
- SetSourceConnection
- SetSourceConnectionLegacy
- getSourceConn

**预计行数**: ~100行  
**风险**: ⭐⭐⭐ (中)  
**原因**: 涉及连接管理，但逻辑相对独立

---

### 第五步：提取核心转发逻辑（最复杂，风险最大）
**目标文件**: `tunnel_bridge_forward.go`
**内容**:
- Start 方法
- copyWithControl 方法
- dynamicSourceWriter 结构体

**预计行数**: ~150行  
**风险**: ⭐⭐⭐⭐ (高)  
**原因**: 这是核心数据转发逻辑，需要仔细测试

---

### 最终保留在 tunnel_bridge.go
**内容**:
- TunnelBridge 结构体定义
- TunnelBridgeConfig 结构体定义
- NewTunnelBridge 构造函数
- Close 方法

**预计行数**: ~180行

---

## 测试策略

每完成一步后：
1. 编译验证：`go build ./internal/protocol/session/...`
2. 重启环境：`bash start_test.sh`
3. 运行测试：`python3 test_mysql.py`
4. 确认测试通过后，继续下一步

---

## 总体改进

- **原文件**: 838行
- **拆分后**: 6个文件，职责更清晰
- **可维护性**: 大幅提升

