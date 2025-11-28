# Tunnox Core 安全与高可用实施路线图

**版本**: v1.0  
**创建日期**: 2025-11-28  
**参考文档**: 
- `ROLLING_UPDATE_FAST_RECOVERY_PLAN.md`
- `TUNNEL_SEAMLESS_MIGRATION_DESIGN.md`
- `RECONNECTION_SECURITY_DESIGN.md`
- `CONNECTION_SECURITY_HARDENING.md`

---

## 总体目标

### 核心目标
1. **安全加固**: 修复指令通道和映射通道的关键安全漏洞
2. **高可用性**: 实现滚动更新时的快速恢复和无感知迁移
3. **可靠性**: 提供隧道重连和状态恢复机制

### 质量目标
- **代码质量**: 遵循既定规范，无重复代码，强类型，Dispose体系
- **测试覆盖**: 核心功能单元测试覆盖率 >= 80%
- **性能影响**: 延迟增加 < 30ms，CPU开销 < 10%

---

## 优先级定义

| 级别 | 说明 | 时间窗口 | 风险等级 |
|------|------|---------|---------|
| **P0** | 严重安全漏洞，必须立即修复 | 1周内 | 严重 |
| **P1** | 重要功能，短期内实施 | 2-3周内 | 高 |
| **P2** | 增强功能，中期规划 | 1-2个月 | 中 |
| **P3** | 优化功能，长期演进 | 按需 | 低 |

---

## Phase 0: 关键安全漏洞修复（P0）

**目标**: 修复现有严重安全漏洞，防止系统被攻击  
**时间**: 1周（5个工作日）  
**依赖**: 无

### 任务列表

#### T0.1 映射通道权限验证（最高优先级）

**问题**: 当前任何客户端知道 SecretKey 即可访问映射，无权限检查

**实施位置**: `internal/app/server/handlers.go`

**任务内容**:
1. 在 `ServerTunnelHandler.HandleTunnelOpen()` 中添加客户端身份提取逻辑
   - 从TLS证书提取 ClientID（如果使用mTLS）
   - 从控制连接映射查找 ClientID
   - 如果无法获取 ClientID，拒绝连接
2. 新增权限验证方法 `hasPermission(clientID, mapping)`
   - 验证 ClientID 是否为 SourceClientID 或 TargetClientID
   - 验证 ClientID 是否属于同一 UserID（如果配置）
3. 权限验证失败时记录安全事件（高风险）

**文件清单**:
- 修改: `internal/app/server/handlers.go` (ServerTunnelHandler)
- 新增: 无（在现有文件中扩展）

**测试要求**:
- 单元测试: `handlers_test.go` - 测试权限验证逻辑
- 测试用例: 
  - 合法客户端（SourceClient/TargetClient）能访问
  - 非法客户端被拒绝
  - 同用户的其他客户端能访问（如果配置）

**预估工作量**: 4小时

---

#### T0.2 暴力破解防护

**问题**: 无认证失败次数限制，可无限尝试密码

**实施位置**: 新增 `internal/security/` 包

**任务内容**:
1. 创建 `BruteForceProtector` 结构体
   - 使用 `map[string]*AttemptRecord` 存储尝试记录（key: "clientID:IP"）
   - 实现 `AllowAttempt(clientID, ip)` 方法
   - 实现 `RecordFailure(clientID, ip)` 方法
   - 实现 `ResetFailures(clientID, ip)` 方法
   - 配置: MaxFailures=5, BlockDuration=15分钟, WindowDuration=5分钟
2. 集成到 `ServerAuthHandler.HandleHandshake()`
   - 认证前检查 `AllowAttempt()`
   - 认证失败记录 `RecordFailure()`
   - 认证成功重置 `ResetFailures()`
3. 定期清理过期记录（使用 dispose 体系）

**文件清单**:
- 新增: `internal/security/brute_force.go` (BruteForceProtector)
- 新增: `internal/security/brute_force_test.go`
- 修改: `internal/app/server/handlers.go` (集成到认证流程)
- 修改: `internal/app/server/server.go` (初始化 BruteForceProtector)

**测试要求**:
- 单元测试: 
  - 5次失败后封禁
  - 封禁期内拒绝请求
  - 时间窗口过期后重置
  - 成功认证后重置计数
- 集成测试: E2E测试暴力破解场景

**预估工作量**: 6小时

---

#### T0.3 IP黑名单机制

**问题**: 无法封禁恶意IP，无法防止DDoS

**实施位置**: 扩展 `internal/security/` 包

**任务内容**:
1. 创建 `IPManager` 结构体
   - 黑名单: `map[string]time.Time` (IP -> 解封时间)
   - 白名单: `map[int64][]string` (ClientID -> 允许的IP列表)
   - 实现 `IsBlocked(ip)` 方法
   - 实现 `BlockIP(ip, duration)` 方法
   - 实现 `IsWhitelisted(clientID, ip)` 方法
   - 支持自动解封（过期后）
2. 集成到连接接受流程
   - 在协议适配器层（`BaseAdapter`）检查IP黑名单
   - 在认证层检查IP白名单（如果客户端配置）
3. 提供管理接口
   - 手动封禁/解封IP
   - 查询黑名单列表

**文件清单**:
- 新增: `internal/security/ip_manager.go`
- 新增: `internal/security/ip_manager_test.go`
- 修改: `internal/protocol/adapter/adapter.go` (集成IP检查)
- 修改: `internal/app/server/handlers.go` (IP白名单验证)
- 新增: `internal/api/handlers_security.go` (管理接口)

**测试要求**:
- 单元测试: 
  - 封禁IP后拒绝连接
  - 自动解封功能
  - 白名单匹配（支持CIDR）
- 集成测试: IP封禁端到端测试

**预估工作量**: 8小时

---

#### T0.4 匿名客户端速率限制

**问题**: 可无限创建匿名客户端，导致资源耗尽

**实施位置**: 扩展 `internal/security/` 包，修改 `handlers.go`

**任务内容**:
1. 创建 `AnonymousRateLimiter` 结构体
   - 基于 `golang.org/x/time/rate` 实现
   - 全局速率: 每秒100个，burst 200
   - 每IP速率: 每分钟3个
2. 添加配置结构 `AnonymousClientConfig`
   - MaxPerIP: 3
   - MaxPerMinute: 100
   - RequireCaptcha: false（可选）
   - DefaultQuota: 映射数1，带宽10MB/s
   - TTL: 24小时
3. 实现匿名客户端配额和过期机制
   - 在 ClientConfig 中添加 QuotaID 和 ExpiresAt 字段
   - 创建 `QuotaManager` 管理配额
   - 定期清理过期的匿名客户端
4. 集成到 `handleAnonymousClient()`

**文件清单**:
- 新增: `internal/security/rate_limiter.go` (AnonymousRateLimiter)
- 新增: `internal/security/rate_limiter_test.go`
- 新增: `internal/cloud/models/quota.go` (QuotaConfig)
- 新增: `internal/cloud/services/quota_service.go` (QuotaManager)
- 新增: `internal/cloud/services/quota_service_test.go`
- 修改: `internal/app/server/config.go` (AnonymousClientConfig)
- 修改: `internal/app/server/handlers.go` (集成速率限制和配额)
- 修改: `internal/cloud/models/client_config.go` (添加QuotaID, ExpiresAt字段)

**测试要求**:
- 单元测试:
  - 速率限制功能
  - 配额检查和限制
  - 过期自动清理
- E2E测试: 匿名客户端创建和配额限制

**预估工作量**: 10小时

---

#### T0.5 隧道速率限制

**问题**: 无隧道创建速率限制，可能被滥用

**实施位置**: 扩展 `internal/security/` 包

**任务内容**:
1. 创建 `TunnelRateLimiter` 结构体
   - 基于 `rate.Limiter`，每个 "clientID:mappingID" 独立限流
   - 配置: 每秒10个隧道，burst 20
2. 集成到 `HandleTunnelOpen()`
3. 添加并发隧道数量限制
   - 在 `SessionManager` 中跟踪活跃隧道数
   - 每个映射最多100个并发隧道

**文件清单**:
- 修改: `internal/security/rate_limiter.go` (添加TunnelRateLimiter)
- 修改: `internal/security/rate_limiter_test.go`
- 修改: `internal/app/server/handlers.go` (集成速率限制)
- 修改: `internal/protocol/session/manager.go` (跟踪活跃隧道数)

**测试要求**:
- 单元测试: 速率限制和并发限制
- 压力测试: 高并发隧道创建

**预估工作量**: 6小时

---

### Phase 0 总结

**总工作量**: 34小时（约5个工作日）  
**关键产出**:
- 新增 `internal/security/` 包（暴力破解、IP管理、速率限制）
- 修复5个严重安全漏洞
- 新增~15个单元测试文件
- 安全事件审计基础设施

**质量保证**:
- ✅ 所有新增代码遵循强类型，避免 `map[string]interface{}`
- ✅ 使用 dispose 体系管理资源（定时清理）
- ✅ 单一职责原则，每个文件职责清晰
- ✅ 100% 单元测试覆盖

---

## Phase 1: 高可用性基础设施（P1）

**目标**: 实现滚动更新快速恢复和基础重连机制  
**时间**: 2周  
**依赖**: Phase 0 完成

### 任务列表

#### T1.1 ServerShutdown 命令实现

**问题**: 服务器关闭时无法通知客户端，导致突然断线

**实施位置**: `internal/packet/`, `internal/protocol/session/`

**任务内容**:
1. 扩展 `packet.CommandType` 枚举
   - 添加 `ServerShutdown CommandType = 15`
2. 定义 `ServerShutdownCommand` 结构体
   - Reason: string (rolling_update, maintenance)
   - GracePeriod: int (秒)
   - RecommendReconnect: bool
3. 在 `SessionManager` 中实现 `BroadcastShutdown()` 方法
   - 遍历所有控制连接
   - 发送 ServerShutdown 命令
4. 集成到优雅关闭流程
   - 在 `server.Stop()` 中调用
   - 在 `ServiceManager.onSignal()` 中集成

**文件清单**:
- 修改: `internal/packet/packet.go` (添加命令类型和结构体)
- 新增: `internal/protocol/session/shutdown.go` (优雅关闭逻辑)
- 新增: `internal/protocol/session/shutdown_test.go`
- 修改: `internal/protocol/session/manager.go` (BroadcastShutdown方法)
- 修改: `internal/app/server/server.go` (集成到Stop流程)

**测试要求**:
- 单元测试: BroadcastShutdown 发送给所有连接
- 集成测试: 收到SIGTERM后客户端接收通知
- E2E测试: 滚动更新场景

**预估工作量**: 8小时

---

#### T1.2 活跃隧道等待机制

**问题**: 服务器关闭时不等待传输完成，导致数据丢失

**实施位置**: `internal/protocol/session/`

**任务内容**:
1. 在 `SessionManager` 中实现隧道跟踪
   - 添加 `GetActiveTunnelCount()` 方法
   - 添加 `WaitForTunnelsToComplete(timeout)` 方法
2. 实现等待逻辑
   - 每500ms检查一次活跃隧道数
   - 如果为0，立即继续
   - 超时后强制继续（记录警告）
3. 集成到优雅关闭流程
   - BroadcastShutdown → WaitForTunnels → CloseConnections

**文件清单**:
- 修改: `internal/protocol/session/shutdown.go` (添加等待逻辑)
- 修改: `internal/protocol/session/shutdown_test.go`
- 修改: `internal/app/server/server.go` (集成到关闭流程)

**测试要求**:
- 单元测试:
  - 无活跃隧道时立即继续
  - 有活跃隧道时等待
  - 超时后强制继续
- E2E测试: 短HTTP请求完整完成

**预估工作量**: 6小时

---

#### T1.3 健康检查端点

**问题**: Nginx无法检测服务器关闭状态，无法提前摘除节点

**实施位置**: `internal/api/`

**任务内容**:
1. 扩展 `/health` 端点
   - 返回状态: "healthy" | "draining" | "unhealthy"
   - draining: 收到关闭信号，不接受新连接
   - 添加详细状态: active_connections, active_tunnels
2. 在服务器中添加健康状态管理
   - 创建 `HealthManager` 管理健康状态
   - 收到SIGTERM后设置为 "draining"
3. 配置Nginx健康检查
   - 提供Nginx配置示例
   - 文档说明如何配置

**文件清单**:
- 新增: `internal/health/manager.go` (HealthManager)
- 新增: `internal/health/manager_test.go`
- 修改: `internal/api/server.go` (扩展健康检查端点)
- 修改: `internal/app/server/server.go` (集成HealthManager)
- 新增: `docs/nginx-health-check.md` (配置文档)
- 修改: `tests/e2e/nginx/load-balancer.conf` (示例配置)

**测试要求**:
- 单元测试: 健康状态切换
- 集成测试: 健康检查API响应
- E2E测试: Nginx健康检查集成

**预估工作量**: 6小时

---

#### T1.4 重连Token机制

**问题**: 重连时无安全凭证，可能被劫持

**实施位置**: 新增 `internal/security/reconnect_token.go`

**任务内容**:
1. 定义 `ReconnectToken` 结构体
   - TokenID, ClientID, TunnelID, NodeID
   - IssuedAt, ExpiresAt (30秒)
   - Nonce (防重放)
   - Signature (HMAC-SHA256)
2. 实现Token生成和验证
   - `GenerateReconnectToken(clientID, tunnelID)`
   - `ValidateReconnectToken(token)` - 多重验证
3. 存储和一次性使用
   - 存储到Redis (key: "reconnect:token:{tokenID}")
   - 验证后立即删除
4. 集成到ServerShutdown
   - 生成Token并随命令发送

**文件清单**:
- 新增: `internal/security/reconnect_token.go`
- 新增: `internal/security/reconnect_token_test.go`
- 修改: `internal/packet/packet.go` (ReconnectToken结构体)
- 修改: `internal/protocol/session/shutdown.go` (生成Token)
- 修改: `internal/app/server/config.go` (Token配置)

**测试要求**:
- 单元测试:
  - Token生成和签名
  - 签名验证
  - 过期检测
  - Nonce防重放
  - 一次性使用
- 安全测试: 重放攻击、篡改攻击

**预估工作量**: 12小时

---

#### T1.5 会话Token机制

**问题**: 认证后无会话Token，无法验证后续请求

**实施位置**: `internal/security/`, `internal/protocol/session/`

**任务内容**:
1. 实现会话Token生成
   - 基于 JWT 或 HMAC
   - 包含: ClientID, IP, TLSFingerprint, IssuedAt, ExpiresAt
2. 在 `SessionManager` 中管理会话
   - `sessionTokens map[int64]string`
   - `sessionExpiry map[int64]time.Time`
   - `ValidateSessionToken(clientID, token)` 方法
   - 自动续期（活动时）
3. 集成到认证流程
   - 认证成功后生成SessionToken
   - 在HandshakeResponse中返回
4. 定期清理过期会话（dispose体系）

**文件清单**:
- 新增: `internal/security/session_token.go`
- 新增: `internal/security/session_token_test.go`
- 修改: `internal/protocol/session/manager.go` (会话管理)
- 修改: `internal/packet/packet.go` (HandshakeResponse添加SessionToken字段)
- 修改: `internal/app/server/handlers.go` (生成和返回Token)

**测试要求**:
- 单元测试:
  - Token生成和验证
  - 会话续期
  - 过期清理
- 集成测试: 认证流程返回SessionToken

**预估工作量**: 8小时

---

#### T1.6 TLS指纹绑定

**问题**: 会话劫持风险

**实施位置**: `internal/security/`

**任务内容**:
1. 实现TLS指纹提取
   - `extractTLSFingerprint(conn *tls.Conn) string`
   - 基于: TLS版本 + 密码套件 + 证书哈希
2. 在认证时存储指纹
   - Redis: "client:{clientID}:tls_fingerprint"
3. 在后续连接时验证指纹
   - 指纹不匹配 → 可疑活动 → 拒绝或MFA

**文件清单**:
- 新增: `internal/security/tls_fingerprint.go`
- 新增: `internal/security/tls_fingerprint_test.go`
- 修改: `internal/app/server/handlers.go` (集成指纹绑定)

**测试要求**:
- 单元测试: 指纹提取和验证
- 安全测试: 会话劫持检测

**预估工作量**: 6小时

---

#### T1.7 安全审计日志

**问题**: 无安全事件记录，无法追踪攻击

**实施位置**: 新增 `internal/security/audit/`

**任务内容**:
1. 定义 `SecurityEvent` 结构体
   - Timestamp, Type, ClientID, IPAddress
   - Success, ErrorReason, RiskScore (1-10)
2. 创建 `SecurityLogger`
   - 记录到专门的日志文件
   - 存储到数据库（可选，使用PostgreSQL）
   - 高风险事件触发告警
3. 集成到所有安全相关操作
   - 认证成功/失败
   - 隧道打开
   - 权限拒绝
   - 速率限制触发
   - IP封禁

**文件清单**:
- 新增: `internal/security/audit/logger.go`
- 新增: `internal/security/audit/logger_test.go`
- 新增: `internal/security/audit/event.go` (SecurityEvent定义)
- 修改: 所有安全相关文件（集成审计）
- 新增: `docs/security-audit.md` (审计日志文档)

**测试要求**:
- 单元测试: 日志记录功能
- 集成测试: 端到端审计日志生成

**预估工作量**: 8小时

---

### Phase 1 总结

**总工作量**: 54小时（约2周）  
**关键产出**:
- ServerShutdown命令和优雅关闭机制
- 健康检查端点（支持Nginx集成）
- 重连Token和会话Token机制
- TLS指纹绑定
- 完整的安全审计体系

**质量保证**:
- ✅ 新增代码遵循强类型
- ✅ 使用 dispose 体系（会话清理、Token清理）
- ✅ 文件职责清晰（security/、health/、audit/ 包分离）
- ✅ 单元测试覆盖 >= 80%

---

## Phase 2: 隧道无感知迁移（P1）

**目标**: 实现隧道中断时的数据缓冲和重连恢复  
**时间**: 2-3周  
**依赖**: Phase 1 完成

### 任务列表

#### T2.1 TunnelData序列号扩展

**问题**: 当前TunnelData无序列号，无法实现可靠传输

**实施位置**: `internal/packet/`

**任务内容**:
1. 扩展 `TransferPacket` 结构
   - 添加 SeqNum, AckNum, Flags 字段
   - 定义 Flags: SYN, FIN, ACK, RST
2. 保持向后兼容
   - 旧格式: 无序列号字段（Flags=0）
   - 新格式: Flags非0时启用序列号
3. 修改序列化/反序列化逻辑

**文件清单**:
- 修改: `internal/packet/packet.go`
- 修改: `internal/stream/packet_stream.go` (序列化逻辑)
- 新增: `internal/packet/packet_v2_test.go`

**测试要求**:
- 单元测试: 序列号序列化/反序列化
- 兼容性测试: 旧客户端与新服务器互操作

**预估工作量**: 8小时

---

#### T2.2 发送端缓冲机制

**问题**: 无发送缓冲，无法重传丢失数据

**实施位置**: 新增 `internal/protocol/session/tunnel_buffer.go`

**任务内容**:
1. 创建 `TunnelSendBuffer` 结构体
   - `buffer map[uint64][]byte` (seqNum -> data)
   - `nextSeq`, `confirmedSeq` 字段
   - MaxBufferSize: 10MB（可配置）
2. 实现方法
   - `Send(data) (seqNum, error)` - 缓冲并发送
   - `ConfirmUpTo(ackNum)` - 清理已确认数据
   - `ResendUnconfirmed() []Packet` - 重传未确认数据
3. 集成到隧道桥接
   - 在 `TunnelBridge` 中使用发送缓冲

**文件清单**:
- 新增: `internal/protocol/session/tunnel_buffer.go`
- 新增: `internal/protocol/session/tunnel_buffer_test.go`
- 修改: `internal/protocol/session/tunnel_bridge.go`

**测试要求**:
- 单元测试:
  - 缓冲和确认机制
  - 缓冲区满处理
  - 重传功能
- 集成测试: 端到端数据传输

**预估工作量**: 12小时

---

#### T2.3 接收端重组机制

**问题**: 无接收缓冲，无法处理乱序包

**实施位置**: `internal/protocol/session/tunnel_buffer.go`

**任务内容**:
1. 创建 `TunnelReceiveBuffer` 结构体
   - `buffer map[uint64][]byte` (乱序包缓冲)
   - `nextExpected uint64` (期望序号)
   - MaxOutOfOrder: 100（最大乱序包数）
2. 实现方法
   - `Receive(pkt) ([]byte, error)` - 接收并重组
   - 处理乱序包缓冲
   - 返回连续数据
3. 集成到隧道桥接

**文件清单**:
- 修改: `internal/protocol/session/tunnel_buffer.go`
- 修改: `internal/protocol/session/tunnel_buffer_test.go`
- 修改: `internal/protocol/session/tunnel_bridge.go`

**测试要求**:
- 单元测试:
  - 顺序包处理
  - 乱序包缓冲和重组
  - 序号跳跃检测
- 集成测试: 模拟乱序网络

**预估工作量**: 10小时

---

#### T2.4 隧道状态持久化

**问题**: 状态仅在内存，服务器切换后丢失

**实施位置**: `internal/protocol/session/`

**任务内容**:
1. 定义 `TunnelState` 结构体
   - TunnelID, MappingID, SourceClientID, TargetClientID
   - LastSeqNum, LastAckNum
   - UpdatedAt, Signature (HMAC)
2. 实现状态存储和加载
   - 存储到 Redis: "tunnel:state:{tunnelID}"
   - TTL: 5分钟
   - 状态签名防篡改
3. 定期更新隧道状态
   - 每传输1000个包更新一次
   - 或每10秒更新一次

**文件清单**:
- 新增: `internal/protocol/session/tunnel_state.go`
- 新增: `internal/protocol/session/tunnel_state_test.go`
- 修改: `internal/protocol/session/tunnel_bridge.go` (集成状态持久化)

**测试要求**:
- 单元测试:
  - 状态存储和加载
  - 签名验证
  - 过期清理
- 集成测试: 跨节点状态恢复

**预估工作量**: 10小时

---

#### T2.5 TunnelReconnect命令

**问题**: 无重连协议，无法恢复中断的隧道

**实施位置**: `internal/packet/`, `internal/protocol/session/`

**任务内容**:
1. 添加 `TunnelReconnect` 命令类型
   - CommandType = 36
2. 定义 `TunnelReconnectRequest` 结构体
   - TunnelID, ReconnectToken
   - LastSentSeq, LastAckSeq
3. 实现重连处理逻辑
   - 验证ReconnectToken
   - 加载TunnelState
   - 对比序列号
   - 重传丢失数据
   - 继续传输
4. 客户端重连逻辑（如果维护客户端）

**文件清单**:
- 修改: `internal/packet/packet.go`
- 新增: `internal/protocol/session/tunnel_reconnect.go`
- 新增: `internal/protocol/session/tunnel_reconnect_test.go`

**测试要求**:
- 单元测试: 重连流程
- E2E测试: 隧道中断和恢复

**预估工作量**: 12小时

---

#### T2.6 客户端HTTP重试机制（如果维护客户端）

**问题**: HTTP请求失败时无自动重试

**实施位置**: 客户端代码（如果在此项目中）

**任务内容**:
1. 实现 HTTP 代理重试逻辑
   - MaxRetries: 3
   - RetryDelay: 100ms, 200ms, 400ms（指数退避）
2. 判断可重试错误
   - 连接断开、502、503、504
   - 非幂等请求（POST等）谨慎重试
3. 超时处理

**文件清单**:
- 修改: 客户端HTTP映射处理器（如果存在）

**测试要求**:
- 单元测试: 重试逻辑
- E2E测试: HTTP请求自动恢复

**预估工作量**: 8小时（如果适用）

---

### Phase 2 总结

**总工作量**: 52-60小时（约2-3周）  
**关键产出**:
- 隧道序列号和缓冲机制
- 发送端和接收端缓冲器
- 隧道状态持久化
- TunnelReconnect重连协议
- HTTP自动重试（可选）

**质量保证**:
- ✅ 强类型：`TunnelSendBuffer`, `TunnelReceiveBuffer`（非 map）
- ✅ Dispose体系：缓冲区清理、状态过期
- ✅ 文件分离：tunnel_buffer.go, tunnel_state.go, tunnel_reconnect.go
- ✅ 单元测试覆盖 >= 85%

---

## Phase 3: 高级安全和监控（P2）

**目标**: 增强安全性和可观测性  
**时间**: 1-2个月  
**依赖**: Phase 1完成

### 任务列表

#### T3.1 并发连接限制

**位置**: `internal/protocol/session/manager.go`

**任务内容**:
1. 限制每个客户端的最大连接数（默认3）
2. 集成到连接接受流程

**预估工作量**: 4小时

---

#### T3.2 异常行为检测

**位置**: 新增 `internal/security/anomaly/`

**任务内容**:
1. 创建 `AnomalyDetector`
   - 建立客户端基线（常用IP、时间段）
   - 检测异常模式
2. 集成到认证流程

**预估工作量**: 16小时

---

#### T3.3 配额管理完善

**位置**: `internal/cloud/services/quota_service.go`

**任务内容**:
1. 完善配额检查逻辑
2. 实时带宽限制
3. 配额用量统计

**预估工作量**: 12小时

---

#### T3.4 多因子认证（MFA）

**位置**: `internal/security/mfa/`

**任务内容**:
1. 支持TOTP（Time-based OTP）
2. 可选启用（针对高安全性客户端）

**预估工作量**: 20小时

---

#### T3.5 Prometheus指标集成

**位置**: 新增 `internal/metrics/`

**任务内容**:
1. 暴露安全相关指标
   - 认证失败次数
   - IP封禁次数
   - 速率限制触发次数
2. 集成Prometheus导出器

**预估工作量**: 16小时

---

### Phase 3 总结

**总工作量**: 68小时（约1-2个月，可并行）  
**关键产出**: 高级安全特性和监控能力

---

## Phase 4: 高级优化（P3）

**目标**: 性能优化和高级特性  
**时间**: 按需  
**依赖**: Phase 2完成

### 任务列表

#### T4.1 智能缓冲策略

**内容**: 根据网络状况动态调整缓冲区大小

**预估工作量**: 24小时

---

#### T4.2 拥塞控制

**内容**: 实现类似TCP的拥塞控制算法

**预估工作量**: 32小时

---

#### T4.3 QUIC深度集成

**内容**: 利用QUIC的原生连接迁移特性

**预估工作量**: 80小时

---

#### T4.4 客户端TLS证书（mTLS）

**内容**: 强制客户端证书认证

**预估工作量**: 16小时

---

### Phase 4 总结

**总工作量**: 152小时（按需实施）

---

## 代码质量保证措施

### 文件组织规范

#### 新增包结构
```
internal/
├── security/              # 安全组件（新增）
│   ├── brute_force.go     # 暴力破解防护
│   ├── ip_manager.go      # IP黑白名单
│   ├── rate_limiter.go    # 速率限制器
│   ├── reconnect_token.go # 重连Token
│   ├── session_token.go   # 会话Token
│   ├── tls_fingerprint.go # TLS指纹
│   ├── audit/             # 安全审计
│   │   ├── logger.go
│   │   └── event.go
│   └── anomaly/           # 异常检测（P2）
│       └── detector.go
├── health/                # 健康检查（新增）
│   └── manager.go
├── protocol/session/
│   ├── shutdown.go        # 优雅关闭（新增）
│   ├── tunnel_buffer.go   # 隧道缓冲（新增）
│   ├── tunnel_state.go    # 状态管理（新增）
│   └── tunnel_reconnect.go # 重连协议（新增）
└── cloud/services/
    └── quota_service.go   # 配额管理（新增）
```

### 文件大小限制
- 单个文件不超过 500 行
- 超过则拆分（如 security/ 包按功能拆分多个文件）

### 类型安全规范
- ❌ 禁止: `map[string]interface{}`, `interface{}`, `any`
- ✅ 使用: 明确的结构体类型
- 示例: `BruteForceProtector` 使用 `map[string]*AttemptRecord`

### Dispose体系遵循
所有需要清理的组件必须：
1. 继承 `dispose.ServiceBase` 或 `dispose.Dispose`
2. 实现 `Close() error` 方法
3. 注册到父 Context
4. 使用定时器时通过 `<-ctx.Done()` 退出

示例组件：
- `BruteForceProtector` - 定期清理过期记录
- `IPManager` - 定期清理过期黑名单
- `SessionManager` - 定期清理过期会话
- `TunnelSendBuffer` - 清理已确认数据

### 命名规范
- 包名: 小写，单数形式（security, health, audit）
- 文件名: 蛇形命名（brute_force.go, tunnel_buffer.go）
- 类型名: 驼峰命名（BruteForceProtector, TunnelSendBuffer）
- 方法名: 驼峰命名（RecordFailure, ConfirmUpTo）
- 常量: 大写下划线（MAX_FAILURES, BLOCK_DURATION）

### 无重复代码原则
- IP检查逻辑统一在 `IPManager`
- Token验证逻辑统一在 `*Token` 文件
- 缓冲逻辑统一在 `TunnelBuffer`
- 不在多处重复实现相同逻辑

---

## 测试覆盖要求

### 单元测试

#### 必须覆盖（100%）
- 所有 `security/` 包中的组件
- 所有 Token 生成和验证逻辑
- 所有缓冲和重组逻辑
- 所有状态管理逻辑

#### 测试文件命名
- 实现文件: `brute_force.go`
- 测试文件: `brute_force_test.go`（同目录）

#### 测试用例要求
每个功能至少包含：
1. **正常流程测试** (Happy Path)
2. **边界条件测试** (Edge Cases)
3. **错误处理测试** (Error Cases)
4. **并发安全测试** (Concurrent Access)

示例：`BruteForceProtector`
- ✅ 正常: 5次失败后封禁
- ✅ 边界: 第4次失败未封禁，第5次立即封禁
- ✅ 错误: 无效IP处理
- ✅ 并发: 多协程同时记录失败

### 集成测试

#### E2E测试场景（Phase 0-1）
1. **安全测试**
   - 暴力破解攻击被阻止
   - 未授权隧道访问被拒绝
   - IP封禁功能有效
2. **高可用测试**
   - 滚动更新场景
   - 短HTTP请求完整完成
   - 健康检查集成
3. **重连测试**
   - 服务器关闭后客户端收到通知
   - ReconnectToken有效性

#### E2E测试场景（Phase 2）
1. **隧道恢复测试**
   - 传输中断后自动重连
   - 数据完整性验证
   - 序列号正确性

### 性能测试

#### 基准测试（Benchmark）
- 暴力破解检查性能（< 1ms）
- IP黑名单检查性能（< 1ms）
- Token验证性能（< 5ms）
- 缓冲操作性能（< 100μs）

#### 压力测试
- 1000个并发客户端认证
- 10000个并发隧道创建
- 持续24小时稳定性测试

---

## 配置管理

### 配置文件结构

在 `internal/app/server/config.go` 中新增：

```yaml
security:
  # 传输层
  tls:
    force: true
    min_version: "1.2"
    require_client_cert: false
  
  # 匿名客户端
  anonymous:
    enabled: true
    max_per_ip: 3
    max_per_minute: 100
    require_captcha: false
    default_quota:
      max_mappings: 1
      max_bandwidth: 10
      max_connections: 10
    ttl: 24h
  
  # 认证
  authentication:
    max_failures: 5
    block_duration: 15m
    session_timeout: 24h
    require_mfa: false
  
  # 速率限制
  rate_limit:
    auth_per_ip_per_minute: 60
    tunnels_per_client_per_second: 10
    global_limit_per_second: 1000
  
  # 连接限制
  connection:
    max_per_client: 3
    max_tunnels_per_mapping: 100
  
  # 审计
  audit:
    enabled: true
    log_file: "/var/log/tunnox/security.log"
    database: false
    alert_on_high_risk: true

# 优雅关闭
graceful_shutdown:
  timeout: 30s
  wait_for_tunnels: true
  max_tunnel_wait: 10s

# 健康检查
health:
  endpoint: "/health"
  detailed: true
```

### 环境变量覆盖
支持所有配置通过环境变量覆盖，格式：
- `SECURITY_TLS_FORCE=true`
- `SECURITY_ANONYMOUS_MAX_PER_IP=5`
- `GRACEFUL_SHUTDOWN_TIMEOUT=60s`

---

## 文档要求

### 必须文档
1. **API文档**: 新增的管理接口（IP管理、配额查询）
2. **配置文档**: 安全配置详细说明
3. **部署文档**: Nginx健康检查配置示例
4. **安全文档**: 安全最佳实践和审计日志格式
5. **迁移指南**: 从当前版本升级的步骤

### 文档位置
- `docs/security-guide.md` - 安全配置指南
- `docs/nginx-health-check.md` - 健康检查配置
- `docs/graceful-shutdown.md` - 优雅关闭使用说明
- `docs/tunnel-migration.md` - 隧道迁移机制
- `docs/audit-log-format.md` - 审计日志格式
- `docs/migration-v1-to-v2.md` - 升级指南

---

## 实施时间表

### 第1周（Phase 0.1-0.3）
- 周一-周二: T0.1 映射权限验证（4h）+ T0.2 暴力破解（6h）
- 周三-周四: T0.3 IP黑名单（8h）+ T0.4 匿名限制开始（5h）
- 周五: T0.4 完成（5h）+ 测试和修复

### 第2周（Phase 0.4-Phase 1.1）
- 周一-周二: T0.5 隧道速率限制（6h）+ Phase 0测试（10h）
- 周三-周五: T1.1 ServerShutdown（8h）+ T1.2 隧道等待（6h）+ T1.3 健康检查（6h）

### 第3周（Phase 1.2-1.7）
- 周一-周三: T1.4 重连Token（12h）+ T1.5 会话Token（8h）
- 周四-周五: T1.6 TLS指纹（6h）+ T1.7 审计日志（8h）

### 第4-6周（Phase 2）
- 第4周: T2.1-T2.2（序列号+发送缓冲）
- 第5周: T2.3-T2.4（接收缓冲+状态持久化）
- 第6周: T2.5-T2.6（重连协议+HTTP重试）+ 测试

### 第7周以后（Phase 3）
- 按优先级排期，可与Phase 2并行

---

## 风险和依赖

### 技术风险
1. **序列号机制复杂度** - 需要充分测试，确保无数据丢失
2. **性能影响** - 缓冲和签名验证可能影响性能，需性能测试
3. **向后兼容** - 新协议需兼容旧客户端

### 缓解措施
1. 分阶段实施，每个阶段充分测试
2. 提供配置开关，可选启用新特性
3. 性能基准测试，确保影响在可接受范围
4. 灰度发布，先在少量节点试运行

### 外部依赖
1. **Redis** - 必须，用于状态持久化和Token存储
2. **PostgreSQL** - 可选，用于审计日志存储
3. **Nginx** - 健康检查集成
4. **监控系统** - 可选，用于告警（Prometheus/Grafana）

---

## 成功指标

### Phase 0成功标准
- ✅ 所有P0安全漏洞修复
- ✅ 单元测试覆盖率 >= 80%
- ✅ 无linter错误
- ✅ 安全测试通过（暴力破解、越权访问）

### Phase 1成功标准
- ✅ 滚动更新时短HTTP请求（< 10s）无感知
- ✅ ServerShutdown通知100%送达
- ✅ 健康检查集成到Nginx
- ✅ 重连Token机制通过安全测试

### Phase 2成功标准
- ✅ 隧道传输中断后能恢复（数据完整性100%）
- ✅ 长连接（> 10s）传输不中断
- ✅ 文件传输支持断点续传
- ✅ 性能影响 < 30ms延迟

### 整体成功标准
- ✅ E2E测试通过率 100%
- ✅ 无已知安全漏洞
- ✅ 代码质量符合规范（无弱类型、dispose体系、无重复）
- ✅ 文档完整（安全、部署、API）
- ✅ 生产环境稳定运行30天无重大问题

---

## 总结

### 工作量汇总
| Phase | 优先级 | 工作量 | 时间窗口 |
|-------|--------|--------|---------|
| Phase 0 | P0 | 34小时 | 1周 |
| Phase 1 | P1 | 54小时 | 2周 |
| Phase 2 | P1 | 52-60小时 | 2-3周 |
| Phase 3 | P2 | 68小时 | 1-2个月 |
| Phase 4 | P3 | 152小时 | 按需 |
| **总计** | - | **360-368小时** | **2-4个月** |

### 关键里程碑
1. **第1周结束**: Phase 0完成，安全漏洞修复
2. **第3周结束**: Phase 1完成，滚动更新支持
3. **第6周结束**: Phase 2完成，隧道无感知迁移
4. **第12周结束**: Phase 3完成，生产级安全和监控

### 下一步行动
1. **立即**: 启动 Phase 0 - T0.1 映射权限验证
2. **本周**: 完成 Phase 0 前3个任务
3. **下周**: Phase 0 完整测试和Phase 1 启动
4. **持续**: 每周代码审查，确保质量标准

---

**文档版本**: v1.0  
**最后更新**: 2025-11-28  
**维护者**: Tunnox Core Team

