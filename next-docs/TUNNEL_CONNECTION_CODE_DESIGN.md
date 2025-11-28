# 隧道连接码（Tunnel Connection Code）设计文档

## 📋 设计概述

**核心理念**：通过**全局唯一的一次性连接码**实现安全、灵活的隧道映射授权，无需预先绑定特定ClientID。

**使用场景**：TargetClient希望临时授权外部访问其内网服务。

---

## 🎯 核心简化点（相比原AuthCode设计）

### 1. 去除ClientID绑定
- ✅ 连接码本身全局唯一，无需预先绑定特定ListenClient
- ✅ 任何知道连接码的Client都可以使用
- ✅ 安全性通过连接码的唯一性、一次性使用和短期有效期保障

### 2. 强制目标地址
- ✅ 连接码必须包含目标地址（如 `tcp://192.168.100.10:8888`）
- ✅ ListenClient使用时，CLI自动解析并显示目标信息
- ✅ 防止连接码被滥用访问其他服务

### 3. 一次性使用
- ✅ 连接码使用后立即失效
- ✅ 但创建的映射继续有效（根据 `MappingDuration`）
- ✅ 大幅降低连接码泄露风险

### 4. 术语更新
- `AuthCode` → **ConnectionCode**（连接码）
- `AccessPermit` → **TunnelMapping**（隧道映射）
- `SourceClient` → **ListenClient**（监听端，使用连接码的客户端）
- `TargetClient` 保持不变（被访问端，生成连接码的客户端）

---

## 🔄 两阶段授权模型

### 阶段1: 连接码（ConnectionCode）

**职责**：临时授权任意客户端建立映射

**特点**：
- **生成者**: TargetClient
- **全局唯一**: 无需绑定特定ListenClient
- **短期有效**: 激活有效期（默认10分钟）
- **一次性使用**: 使用后立即失效
- **必须包含目标地址**: 如 `tcp://192.168.100.10:8888`
- **可撤销**: 未使用时可主动撤销

**生命周期**：
```
创建 → [激活有效期: 10分钟] → 被使用/过期/撤销 → 失效
```

### 阶段2: 隧道映射（TunnelMapping）

**职责**：实际的端口映射和流量转发

**特点**：
- **激活者**: ListenClient（任意客户端使用连接码激活）
- **长期有效**: 映射有效期（7天、30天等）
- **绑定ListenClient**: 防止映射被劫持
- **可被撤销**: TargetClient或ListenClient都可撤销
- **使用统计**: 跟踪连接次数、流量等

**生命周期**：
```
激活创建 → [映射有效期: 7天] → 过期/撤销 → 失效
```

---

## 📊 数据模型

### TunnelConnectionCode（连接码）

```go
// TunnelConnectionCode 隧道连接码
// 由TargetClient生成，用于授权任意客户端建立映射
// 一次性使用，使用后立即失效
type TunnelConnectionCode struct {
    // 基础信息
    ID             string    `json:"id"`               // 连接码ID: conncode_xxx
    Code           string    `json:"code"`             // 好记的连接码（abc-def-123）
    
    // ⭐ 目标信息（必填）
    TargetClientID int64     `json:"target_client_id"` // 生成连接码的TargetClient
    TargetAddress  string    `json:"target_address"`   // ⭐ 必填：tcp://192.168.100.10:8888
    
    // ⭐ 时限控制
    CreatedAt           time.Time     `json:"created_at"`
    ActivationExpiresAt time.Time     `json:"activation_expires_at"` // 激活过期时间（如10分钟）
    ActivationTTL       time.Duration `json:"activation_ttl"`        // 激活有效期
    MappingDuration     time.Duration `json:"mapping_duration"`      // 激活后映射有效期（如7天）
    
    // ⭐ 使用控制（一次性）
    IsActivated    bool       `json:"is_activated"`                // 是否已被使用
    ActivatedAt    *time.Time `json:"activated_at,omitempty"`      // 使用时间
    ActivatedBy    *int64     `json:"activated_by,omitempty"`      // 使用该连接码的ListenClientID
    MappingID      *string    `json:"mapping_id,omitempty"`        // 创建的映射ID
    
    // 管理信息
    CreatedBy      string     `json:"created_by"`                  // 创建者（UserID或ClientID）
    IsRevoked      bool       `json:"is_revoked"`                  // 是否已撤销
    RevokedAt      *time.Time `json:"revoked_at,omitempty"`        // 撤销时间
    RevokedBy      string     `json:"revoked_by,omitempty"`        // 撤销者
    Description    string     `json:"description,omitempty"`       // 描述
}

// IsValidForActivation 检查连接码是否可用于激活
func (cc *TunnelConnectionCode) IsValidForActivation() bool {
    return !cc.IsRevoked && 
           !cc.IsActivated && 
           time.Now().Before(cc.ActivationExpiresAt)
}

// CanBeActivatedBy 检查是否可被指定客户端激活
func (cc *TunnelConnectionCode) CanBeActivatedBy(listenClientID int64) bool {
    if !cc.IsValidForActivation() {
        return false
    }
    // ⭐ 不再检查ClientID绑定，任何客户端都可以使用
    return true
}
```

### TunnelMapping（隧道映射）

```go
// TunnelMapping 隧道映射
// 由ListenClient使用连接码激活创建
// 实现 ListenClient → TargetClient 的端口映射
type TunnelMapping struct {
    // 基础信息
    ID               string    `json:"id"`                 // 映射ID: mapping_xxx
    ConnectionCodeID string    `json:"connection_code_id"` // 关联的连接码ID
    
    // ⭐ 映射双方
    ListenClientID int64     `json:"listen_client_id"` // 监听端（使用连接码的客户端）
    TargetClientID int64     `json:"target_client_id"` // 目标端（被访问的客户端）
    
    // ⭐ 地址信息
    ListenAddress  string    `json:"listen_address"`   // ListenClient提供的监听地址（0.0.0.0:9999）
    TargetAddress  string    `json:"target_address"`   // TargetClient的目标地址（tcp://192.168.100.10:8888）
    
    // 时限控制
    CreatedAt      time.Time     `json:"created_at"`
    ExpiresAt      time.Time     `json:"expires_at"`
    Duration       time.Duration `json:"duration"`
    
    // 管理信息
    CreatedBy      string     `json:"created_by"`
    IsRevoked      bool       `json:"is_revoked"`
    RevokedAt      *time.Time `json:"revoked_at,omitempty"`
    RevokedBy      string     `json:"revoked_by,omitempty"`
    
    // ⭐ 使用统计
    LastUsedAt     *time.Time `json:"last_used_at,omitempty"` // 最后使用时间
    UsageCount     int64      `json:"usage_count"`            // 使用次数（连接数）
    BytesSent      int64      `json:"bytes_sent"`             // 发送字节数
    BytesReceived  int64      `json:"bytes_received"`         // 接收字节数
}

// IsValid 检查映射是否有效
func (tm *TunnelMapping) IsValid() bool {
    return !tm.IsRevoked && time.Now().Before(tm.ExpiresAt)
}

// CanBeAccessedBy 检查是否允许访问
func (tm *TunnelMapping) CanBeAccessedBy(clientID int64) bool {
    if !tm.IsValid() {
        return false
    }
    // 只有ListenClient可以使用此映射
    return tm.ListenClientID == clientID
}
```

---

## 🔐 安全设计

### 1. 连接码泄露风险控制

#### 短期有效期（激活窗口）
- **默认**: 10分钟
- **可配置**: 1分钟 ~ 1小时
- **原理**: 大幅缩短泄露后的风险窗口

#### 一次性使用
- **机制**: 使用后立即标记为 `IsActivated=true`
- **效果**: 即使连接码泄露，也只能被使用一次
- **实现**: 激活时原子性检查 `IsActivated` 状态

#### 主动撤销
- **场景**: 连接码分享给错误的人
- **操作**: TargetClient可在未使用前撤销
- **实现**: 设置 `IsRevoked=true`

#### 好记格式
- **格式**: `abc-def-123`（3段，每段3字符）
- **字符集**: 排除易混淆字符（i, l, o）
- **熵值**: 4.6 × 10^13（足够抵抗暴力破解）
- **优势**: 方便口头或文字分享，减少复制错误

### 2. 映射滥用风险控制

#### 绑定ListenClient
- **机制**: 映射创建时绑定 `ListenClientID`
- **效果**: 防止映射被其他客户端劫持
- **验证**: 每次连接时验证 `clientID == mapping.ListenClientID`

#### 使用统计与监控
- **指标**: 连接次数、流量、最后使用时间
- **告警**: 异常流量、高频连接
- **审计**: 完整的使用日志

#### 可撤销机制
- **TargetClient**: 可撤销所有通过其连接码创建的映射
- **ListenClient**: 可撤销自己创建的映射
- **管理员**: 可强制撤销任何映射

#### 有效期限制
- **默认**: 7天
- **可配置**: 1小时 ~ 30天
- **自动过期**: 到期后自动失效

### 3. 暴力破解防护

#### 连接码复杂度
- **格式**: 3段 × 3字符
- **字符集**: 33个字符（0-9, a-z, 排除i/l/o）
- **总空间**: 33^9 ≈ 4.6 × 10^13
- **暴力破解**: 假设每秒1000次尝试，需要1460年

#### 激活失败限制
- **策略**: 同一IP连续失败5次 → 临时封禁10分钟
- **清理**: 成功激活后清零失败计数
- **绕过**: 使用不同IP攻击 → 全局失败计数

#### IP黑名单
- **触发**: 短时间内大量失败尝试
- **持续**: 24小时 ~ 永久
- **解除**: 手动或自动（24小时后）

### 4. 审计与追踪

#### 连接码生命周期日志
```json
{
  "event": "connection_code_created",
  "code_id": "conncode_xxx",
  "code": "abc-def-123",
  "target_client_id": 88888888,
  "target_address": "tcp://192.168.100.10:8888",
  "created_by": "user_123",
  "timestamp": "2025-11-28T12:30:00Z"
}

{
  "event": "connection_code_activated",
  "code_id": "conncode_xxx",
  "code": "abc-def-123",
  "listen_client_id": 77777777,
  "mapping_id": "mapping_xxx",
  "timestamp": "2025-11-28T12:35:00Z"
}

{
  "event": "connection_code_revoked",
  "code_id": "conncode_xxx",
  "revoked_by": "user_123",
  "timestamp": "2025-11-28T12:40:00Z"
}
```

#### 映射使用日志
```json
{
  "event": "mapping_connection",
  "mapping_id": "mapping_xxx",
  "listen_client_id": 77777777,
  "target_client_id": 88888888,
  "bytes_sent": 1024,
  "bytes_received": 2048,
  "duration_ms": 1500,
  "timestamp": "2025-11-28T12:50:00Z"
}
```

---

## 💻 客户端完整设计（连接码 + CLI + 命令行）

---

## 🚀 客户端启动模式

### 模式1: CLI交互模式（默认）

**启动方式**：
```bash
$ tunnox-client
```

**启动流程**：
```
1. 加载配置文件 config.json（如果存在）
   └─ 优先级：./config.json → ~/.tunnox/config.json → /etc/tunnox/config.json
   
2. 解析配置：
   ├─ server_url: "wss://server1.tunnox.io:7000"
   ├─ protocol: "websocket"
   ├─ client_id: 88888888
   ├─ secret_key: "your-secret-key"
   └─ auto_connect: true  (可选，默认false)

3. 显示欢迎界面

4. 如果 auto_connect=true，自动连接服务器

5. 进入CLI交互循环
   └─ 提示符: tunnox>
```

**config.json 示例**（最小化配置）：
```json
{
  "server_url": "wss://server1.tunnox.io:7000",
  "protocol": "websocket",
  "client_id": 88888888,
  "secret_key": "your-secret-key",
  "auto_connect": true,
  "log_level": "info"
}
```

**启动输出**：
```bash
$ tunnox-client
  _____                        
 |_   _|   _ _ __  _ __   _____  __
   | || | | | '_ \| '_ \ / _ \ \/ /
   | || |_| | | | | | | | (_) >  < 
   |_| \__,_|_| |_|_| |_|\___/_/\_\
                                    
Tunnox Client v1.0.0
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Loading config from: ./config.json
✓ Config loaded

Auto-connecting to wss://server1.tunnox.io:7000...
✓ Connected as ClientID: 88888888

Type 'help' for available commands

tunnox>
```

**特点**：
- ✅ 默认启动模式
- ✅ 加载 config.json（仅基础配置）
- ✅ 提供交互式命令行界面
- ✅ 支持Tab补全、历史记录、彩色输出
- ✅ 实时反馈和进度提示

---

### 模式2: 守护进程模式（服务启动）

**启动方式**：
```bash
$ tunnox-client --daemon [options]
```

**启动流程**：
```
1. 解析命令行参数（优先级高于config.json）

2. 后台运行（无CLI界面）

3. 日志输出到文件：~/.tunnox/client.log

4. PID文件：~/.tunnox/client.pid
```

**使用场景**：

#### 场景A: 守护进程持久化运行
```bash
# 后台运行，保持连接
$ tunnox-client --daemon \
    --server wss://server1.tunnox.io:7000 \
    --client-id 88888888 \
    --secret-key your-secret-key \
    --log-file /var/log/tunnox-client.log

[INFO] Tunnox Client started (PID: 12345)
[INFO] Connected to server
```

#### 场景B: 启动时自动建立映射（守护进程 + 映射）
```bash
# 使用连接码建立映射，后台运行
$ tunnox-client --daemon \
    --use-code abc-def-123 \
    --listen 0.0.0.0:9999 \
    --log-file /var/log/tunnox-mapping.log

[INFO] Tunnox Client started (PID: 12346)
[INFO] Activating connection code: abc-def-123
[INFO] Mapping created: mapping_xxx
[INFO] Listening on 0.0.0.0:9999 → tcp://192.168.100.10:8888
```

#### 场景C: 一次性操作（执行后退出）
```bash
# 生成连接码（不启动守护进程）
$ tunnox-client generate-code \
    --target tcp://192.168.100.10:8888 \
    --expire 10m \
    --mapping-duration 7d

🔑 Connection Code: abc-def-123
Target: tcp://192.168.100.10:8888
Activation Period: 10 minutes
Mapping Duration: 7 days
⚠️  One-time use only

# 查看状态（一次性命令）
$ tunnox-client status
Client: 88888888
Server: wss://server1.tunnox.io:7000
Status: Connected
Uptime: 2d 3h 15m
Mappings: 2 active

# 列出映射（一次性命令）
$ tunnox-client list-mappings
Active Mappings (2):
  mapping_xxx: 0.0.0.0:9999 → tcp://192.168.100.10:8888 (6d 23h)
  mapping_yyy: 0.0.0.0:8888 → tcp://10.0.0.5:3306 (2d 5h)
```

**特点**：
- ✅ 无CLI界面（非交互）
- ✅ 后台运行或一次性执行
- ✅ 适合脚本和自动化
- ✅ 命令行参数优先级高于config.json
- ✅ 日志输出到文件

---

## 📝 配置文件设计（config.json）

### 配置原则
✅ **最小化** - 只包含基础连接信息  
✅ **无业务逻辑** - 不包含映射、连接码等运行时数据  
✅ **可选** - 所有配置都可通过命令行参数覆盖  

### 完整配置模板

```json
{
  "// 服务器连接": "================================================",
  "server_url": "wss://server1.tunnox.io:7000",
  "protocol": "websocket",
  
  "// 客户端身份": "================================================",
  "client_id": 88888888,
  "secret_key": "your-secret-key-here",
  
  "// 连接选项": "================================================",
  "auto_connect": true,
  "reconnect": true,
  "reconnect_interval": "5s",
  "heartbeat_interval": "30s",
  
  "// 日志配置": "================================================",
  "log_level": "info",
  "log_file": "",
  
  "// TLS配置（可选）": "================================================",
  "tls": {
    "enabled": true,
    "skip_verify": false,
    "ca_cert": "",
    "client_cert": "",
    "client_key": ""
  },
  
  "// 代理配置（可选）": "================================================",
  "proxy": {
    "enabled": false,
    "type": "socks5",
    "address": "127.0.0.1:1080"
  }
}
```

### 最小配置示例

```json
{
  "server_url": "wss://server1.tunnox.io:7000",
  "client_id": 88888888,
  "secret_key": "your-secret-key",
  "auto_connect": true
}
```

### 配置加载优先级

```
命令行参数 > 环境变量 > config.json > 默认值
```

**示例**：
```bash
# config.json 中 server_url = "wss://server1.tunnox.io:7000"
# 命令行指定不同的服务器
$ tunnox-client --server wss://server2.tunnox.io:7000
# ✓ 使用 server2（命令行优先）
```

---

## 🎮 CLI交互式命令完整列表

### 连接管理

```bash
# 连接到服务器
tunnox> connect <server-url>
tunnox> connect wss://server1.tunnox.io:7000

# 断开连接
tunnox> disconnect

# 重新连接
tunnox> reconnect

# 查看连接状态
tunnox> status
```

### 连接码管理（TargetClient）

```bash
# 生成连接码
tunnox> generate-code \
    --target <address> \
    --expire <duration> \
    --mapping-duration <duration> \
    [--description <text>]

tunnox> generate-code \
    --target tcp://192.168.100.10:8888 \
    --expire 10m \
    --mapping-duration 7d \
    --description "数据库临时访问"

# 列出我生成的连接码
tunnox> list-codes [--status active|used|expired|all]

# 查看连接码详情
tunnox> show-code <code>

# 撤销连接码（未使用时）
tunnox> revoke-code <code>
tunnox> revoke-code abc-def-123
```

### 映射管理

```bash
# 使用连接码创建映射（ListenClient）
tunnox> use-code <code> --listen <address>
tunnox> use-code abc-def-123 --listen 0.0.0.0:9999

# 列出我的映射
tunnox> list-mappings [--type inbound|outbound|all]
# outbound: 我作为ListenClient创建的映射
# inbound:  其他人通过我的连接码创建的映射

# 查看映射详情
tunnox> show-mapping <mapping-id>

# 删除映射
tunnox> delete-mapping <mapping-id>
tunnox> delete-mapping mapping_xxx

# 查看映射统计
tunnox> mapping-stats <mapping-id>
```

### 系统管理

```bash
# 查看帮助
tunnox> help [command]
tunnox> help generate-code

# 查看版本
tunnox> version

# 查看配置
tunnox> config show

# 更新配置（运行时）
tunnox> config set <key> <value>
tunnox> config set auto_connect true

# 查看日志
tunnox> logs [--tail <n>] [--level <level>]
tunnox> logs --tail 50 --level error

# 清屏
tunnox> clear

# 退出
tunnox> exit
tunnox> quit
```

### 调试命令

```bash
# Ping服务器
tunnox> ping

# 查看网络延迟
tunnox> latency

# 查看客户端信息
tunnox> info

# 测试连接码
tunnox> test-code <code>
```

---

## 🔧 命令行参数完整列表

### 全局参数

```bash
--server <url>              # 服务器地址（覆盖config.json）
--protocol <tcp|ws|udp>     # 协议类型
--client-id <id>            # 客户端ID
--secret-key <key>          # 密钥
--config <path>             # 指定配置文件路径
--log-level <level>         # 日志级别：debug|info|warn|error
--log-file <path>           # 日志文件路径
--daemon                    # 守护进程模式
--pid-file <path>           # PID文件路径
--help, -h                  # 显示帮助
--version, -v               # 显示版本
```

### 连接码相关

```bash
# 生成连接码（一次性命令）
tunnox-client generate-code \
    --target <address> \
    --expire <duration> \
    --mapping-duration <duration> \
    [--description <text>]

# 示例
tunnox-client generate-code \
    --target tcp://192.168.100.10:8888 \
    --expire 10m \
    --mapping-duration 7d \
    --description "临时访问"
```

### 映射相关

```bash
# 使用连接码创建映射（守护进程模式）
tunnox-client --daemon \
    --use-code <code> \
    --listen <address>

# 示例
tunnox-client --daemon \
    --use-code abc-def-123 \
    --listen 0.0.0.0:9999
```

### 查询命令（一次性）

```bash
# 查看状态
tunnox-client status

# 列出连接码
tunnox-client list-codes [--status active|used|expired|all]

# 列出映射
tunnox-client list-mappings [--type inbound|outbound|all]

# 查看映射详情
tunnox-client show-mapping <mapping-id>

# 查看映射统计
tunnox-client mapping-stats <mapping-id>
```

### 管理命令（一次性）

```bash
# 撤销连接码
tunnox-client revoke-code <code>

# 删除映射
tunnox-client delete-mapping <mapping-id>

# 停止守护进程
tunnox-client stop [--pid-file <path>]
```

---

## 📋 命令行完整使用示例

### 场景1: 日常开发使用（CLI交互）

```bash
$ tunnox-client
tunnox> connect wss://server1.tunnox.io:7000
✓ Connected as ClientID: 88888888

# TargetClient生成连接码
tunnox> generate-code \
    --target tcp://localhost:3306 \
    --expire 10m \
    --mapping-duration 1d \
    --description "本地MySQL访问"

🔑 Connection Code: db7-m3x-k9p
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Target: tcp://localhost:3306
Activation Period: 10 minutes
Mapping Duration: 1 day
Description: 本地MySQL访问
⚠️  One-time use, expires in 10 minutes
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# 查看我的连接码
tunnox> list-codes
My Connection Codes (3):
┌──────────────┬──────────────────────┬────────┬──────────┐
│ Code         │ Target               │ Status │ Expires  │
├──────────────┼──────────────────────┼────────┼──────────┤
│ db7-m3x-k9p  │ tcp://localhost:3306 │ Active │ 9m 45s   │
│ web-5a2-n8k  │ tcp://localhost:8080 │ Used   │ -        │
│ ssh-7x9-p4m  │ tcp://localhost:22   │ Expired│ -        │
└──────────────┴──────────────────────┴────────┴──────────┘

# 查看谁在访问我
tunnox> list-mappings --type inbound
Inbound Mappings (1):
┌──────────────┬────────────┬──────────────────────┬────────────┐
│ Mapping      │ Client     │ Target               │ Expires    │
├──────────────┼────────────┼──────────────────────┼────────────┤
│ mapping_abc  │ 77777777   │ tcp://localhost:3306 │ 23h 15m    │
└──────────────┴────────────┴──────────────────────┴────────────┘

tunnox> exit
Goodbye!
```

### 场景2: ListenClient使用连接码（CLI交互）

```bash
$ tunnox-client
tunnox> connect wss://server1.tunnox.io:7000
✓ Connected as ClientID: 77777777

tunnox> use-code db7-m3x-k9p --listen 127.0.0.1:3306

🔍 Validating connection code...
📋 Connection Code Info:
   Target: tcp://192.168.100.10:3306
   Mapping Duration: 1 day
   
🔧 Creating mapping...
✓ Mapping created successfully
   Mapping ID: mapping_abc
   Local Listen: 127.0.0.1:3306
   Remote Target: tcp://192.168.100.10:3306
   Expires: 2025-11-29 12:30:00 (23h 59m)
   
💡 You can now connect to localhost:3306

tunnox> list-mappings
My Mappings (1):
┌──────────────┬────────────────┬──────────────────────────┬────────────┐
│ Mapping      │ My Listen      │ Remote Target            │ Expires    │
├──────────────┼────────────────┼──────────────────────────┼────────────┤
│ mapping_abc  │ 127.0.0.1:3306 │ tcp://192.168.100.10:... │ 23h 59m    │
└──────────────┴────────────────┴──────────────────────────┴────────────┘

# 连接本地MySQL
$ mysql -h 127.0.0.1 -P 3306 -u user -p
```

### 场景3: 服务器部署（守护进程）

```bash
# 启动守护进程，使用连接码建立映射
$ tunnox-client --daemon \
    --server wss://server1.tunnox.io:7000 \
    --use-code abc-def-123 \
    --listen 0.0.0.0:9999 \
    --log-file /var/log/tunnox/mapping.log \
    --pid-file /var/run/tunnox-client.pid

[2025-11-28 12:30:00] INFO: Tunnox Client starting...
[2025-11-28 12:30:01] INFO: Connected to wss://server1.tunnox.io:7000
[2025-11-28 12:30:01] INFO: ClientID: 77777777
[2025-11-28 12:30:02] INFO: Activating connection code: abc-def-123
[2025-11-28 12:30:02] INFO: Mapping created: mapping_xxx
[2025-11-28 12:30:02] INFO: Listening on 0.0.0.0:9999
[2025-11-28 12:30:02] INFO: Forwarding to tcp://192.168.100.10:8888
[2025-11-28 12:30:02] INFO: Daemon started (PID: 12345)

# 查看状态
$ tunnox-client status
Client: 77777777
Server: wss://server1.tunnox.io:7000
Status: Connected
Uptime: 2m 30s
Mappings: 1 active

# 停止守护进程
$ tunnox-client stop
Stopping Tunnox Client (PID: 12345)...
✓ Client stopped
```

### 场景4: 自动化脚本

```bash
#!/bin/bash
# deploy-mapping.sh

# 生成连接码
CODE=$(tunnox-client generate-code \
    --target tcp://192.168.1.100:5432 \
    --expire 5m \
    --mapping-duration 7d \
    --output json | jq -r '.code')

echo "Connection Code: $CODE"

# 分发给需要访问的服务器（通过安全渠道）
ssh remote-server "tunnox-client --daemon \
    --use-code $CODE \
    --listen 0.0.0.0:5432 \
    --log-file /var/log/tunnox.log"

echo "Mapping deployed successfully"
```

---

## 🎯 CLI vs 命令行对比

| 特性 | CLI交互模式 | 命令行模式 |
|------|------------|-----------|
| **启动方式** | `tunnox-client` | `tunnox-client <command>` |
| **交互性** | ✅ 交互式提示符 | ❌ 一次性执行 |
| **适用场景** | 日常开发、调试 | 脚本、自动化、守护进程 |
| **配置加载** | ✅ 自动加载config.json | ✅ 加载config.json（可覆盖） |
| **输出格式** | 彩色、表格、友好 | 简洁、易解析（支持JSON） |
| **错误处理** | 友好提示，继续运行 | 返回错误码，退出进程 |
| **Tab补全** | ✅ 支持 | ❌ 不适用 |
| **历史记录** | ✅ 支持（~/.tunnox/history） | ❌ 不适用 |

---

## 📚 CLI实现技术栈

### 推荐库

1. **readline** - 交互式命令行
   - Tab补全
   - 历史记录
   - 行编辑

2. **cobra** - 命令行框架
   - 子命令管理
   - 参数解析
   - 帮助生成

3. **viper** - 配置管理
   - 多格式支持（JSON/YAML/TOML）
   - 环境变量
   - 优先级管理

4. **tablewriter** - 表格输出
   - 对齐、边框
   - 彩色输出

5. **logrus** - 日志
   - 结构化日志
   - 多级别
   - 格式化

---

## 🔄 启动流程图

```
┌─────────────────────────────────────────────────────────┐
│                    tunnox-client 启动                     │
└─────────────────────────────────────────────────────────┘
                          │
                          ├─ 检查命令行参数
                          │
              ┌───────────┴───────────┐
              │                       │
         有子命令                  无子命令
              │                       │
              ▼                       ▼
    ┌──────────────────┐    ┌──────────────────┐
    │  命令行模式       │    │  CLI交互模式      │
    └──────────────────┘    └──────────────────┘
              │                       │
              │                       ├─ 加载config.json
              │                       │
              ▼                       ├─ 显示欢迎界面
    ┌──────────────────┐              │
    │ 一次性命令        │              ├─ 自动连接（如果配置）
    │ - generate-code  │              │
    │ - list-mappings  │              ▼
    │ - status         │    ┌──────────────────┐
    └──────────────────┘    │  进入CLI循环      │
              │              │  tunnox>         │
              ▼              └──────────────────┘
         执行并退出                   │
                                     ├─ 读取命令
                                     │
                                     ├─ 解析并执行
                                     │
                                     ├─ 显示结果
                                     │
                                     └─ 循环（直到exit）
```

---

## 🎨 CLI命令详细设计

### 1. generate-code - 生成连接码

**语法**：
```bash
tunnox> generate-code \
    --target <protocol://host:port> \
    --expire <duration> \
    --mapping-duration <duration> \
    [--description <text>] \
    [--output json|table]
```

**参数说明**：
- `--target`: 目标地址（必填），格式：`tcp://192.168.100.10:8888`
- `--expire`: 激活有效期（必填），如 `10m`, `1h`
- `--mapping-duration`: 映射有效期（必填），如 `1d`, `7d`, `30d`
- `--description`: 描述（可选）
- `--output`: 输出格式（可选），默认 `table`

**输出示例**：

表格格式：
```bash
tunnox> generate-code \
    --target tcp://192.168.100.10:8888 \
    --expire 10m \
    --mapping-duration 7d \
    --description "临时数据库访问"

🔑 Connection Code Generated
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Code:              abc-def-123
Target:            tcp://192.168.100.10:8888
Activation TTL:    10 minutes
Mapping Duration:  7 days
Created:           2025-11-28 12:30:00
Expires:           2025-11-28 12:40:00
Status:            Active
Description:       临时数据库访问
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
⚠️  One-time use only
⚠️  Share securely, expires in 10 minutes
```

JSON格式：
```bash
tunnox> generate-code --target tcp://localhost:3306 --expire 10m --mapping-duration 1d --output json

{
  "success": true,
  "data": {
    "id": "conncode_abc123",
    "code": "db7-m3x-k9p",
    "target_address": "tcp://localhost:3306",
    "activation_ttl": "10m",
    "mapping_duration": "24h",
    "created_at": "2025-11-28T12:30:00Z",
    "expires_at": "2025-11-28T12:40:00Z",
    "status": "active"
  }
}
```

---

### 2. use-code - 使用连接码创建映射

**语法**：
```bash
tunnox> use-code <code> --listen <address> [--output json|table]
```

**参数说明**：
- `<code>`: 连接码（必填）
- `--listen`: 本地监听地址（必填），格式：`0.0.0.0:9999` 或 `127.0.0.1:9999`

**执行流程**：
```
1. 验证连接码有效性
   ├─ 检查是否存在
   ├─ 检查是否已使用
   ├─ 检查是否过期
   └─ 检查是否已撤销

2. 显示连接码信息（目标地址等）

3. 创建隧道映射
   ├─ 调用API: POST /api/connection-codes/{code}/activate
   ├─ 获取MappingID
   └─ 连接码标记为已使用

4. 显示映射详情

5. 开始本地监听（如果在守护进程模式）
```

**输出示例**：
```bash
tunnox> use-code db7-m3x-k9p --listen 127.0.0.1:3306

🔍 Validating connection code...
✓ Connection code is valid

📋 Connection Code Information:
   Code:              db7-m3x-k9p
   Target:            tcp://192.168.100.10:3306
   Mapping Duration:  1 day
   Description:       本地MySQL访问
   
🔧 Creating tunnel mapping...
✓ Mapping created successfully

📍 Mapping Details:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Mapping ID:        mapping_abc123
My Listen:         127.0.0.1:3306
Remote Target:     tcp://192.168.100.10:3306
Status:            Active
Expires:           2025-11-29 12:30:00 (23h 59m)
Created:           2025-11-28 12:30:15
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

💡 You can now connect to:
   $ mysql -h 127.0.0.1 -P 3306

⚠️  Connection code 'db7-m3x-k9p' has been consumed (one-time use)
```

**错误处理示例**：
```bash
tunnox> use-code invalid-code --listen 0.0.0.0:9999

❌ Error: Connection code not found or invalid
   Code: invalid-code
   
tunnox> use-code abc-def-123 --listen 0.0.0.0:9999

❌ Error: Connection code already used
   Code: abc-def-123
   Used by: ClientID 77777777
   Used at: 2025-11-28 12:00:00
   Mapping: mapping_xyz789
```

---

### 3. list-codes - 列出连接码

**语法**：
```bash
tunnox> list-codes [--status active|used|expired|revoked|all] [--output json|table]
```

**输出示例**：
```bash
tunnox> list-codes --status all

My Connection Codes (5):
┌──────────────┬────────────────────────────┬─────────┬────────────┬────────────────┐
│ Code         │ Target                     │ Status  │ Expires    │ Used By        │
├──────────────┼────────────────────────────┼─────────┼────────────┼────────────────┤
│ db7-m3x-k9p  │ tcp://localhost:3306       │ Active  │ 9m 45s     │ -              │
│ web-5a2-n8k  │ tcp://localhost:8080       │ Used    │ -          │ Client 7777... │
│ ssh-7x9-p4m  │ tcp://localhost:22         │ Expired │ -          │ -              │
│ api-2k8-v3n  │ tcp://10.0.0.5:5000        │ Revoked │ -          │ -              │
│ mq-9p4-x7a   │ tcp://192.168.1.100:5672   │ Active  │ 3m 20s     │ -              │
└──────────────┴────────────────────────────┴─────────┴────────────┴────────────────┘

Total: 5 codes (2 active, 1 used, 1 expired, 1 revoked)

# 只看活跃的
tunnox> list-codes --status active

Active Connection Codes (2):
┌──────────────┬────────────────────────────┬──────────┐
│ Code         │ Target                     │ Expires  │
├──────────────┼────────────────────────────┼──────────┤
│ db7-m3x-k9p  │ tcp://localhost:3306       │ 9m 45s   │
│ mq-9p4-x7a   │ tcp://192.168.1.100:5672   │ 3m 20s   │
└──────────────┴────────────────────────────┴──────────┘
```

---

### 4. list-mappings - 列出映射

**语法**：
```bash
tunnox> list-mappings [--type inbound|outbound|all] [--output json|table]
```

**类型说明**：
- `outbound`: 我作为ListenClient创建的映射（我在访问别人）
- `inbound`: 别人通过我的连接码创建的映射（别人在访问我）
- `all`: 全部映射

**输出示例**：

Outbound映射（我的映射）：
```bash
tunnox> list-mappings --type outbound

My Outbound Mappings (2):
┌──────────────┬────────────────┬────────────────────────────┬──────────┬────────────┐
│ Mapping ID   │ My Listen      │ Remote Target              │ Status   │ Expires    │
├──────────────┼────────────────┼────────────────────────────┼──────────┼────────────┤
│ mapping_abc  │ 127.0.0.1:3306 │ tcp://192.168.100.10:3306  │ Active   │ 6d 23h     │
│ mapping_xyz  │ 0.0.0.0:8080   │ tcp://10.0.0.5:8080        │ Active   │ 2d 5h      │
└──────────────┴────────────────┴────────────────────────────┴──────────┴────────────┘

Total: 2 mappings
Usage: 45 connections, 2.3 GB transferred
```

Inbound映射（谁在访问我）：
```bash
tunnox> list-mappings --type inbound

Inbound Mappings (Who's Accessing Me):
┌──────────────┬────────────┬────────────────┬────────────────────────┬──────────┬────────────┐
│ Mapping ID   │ Client ID  │ Their Listen   │ My Target              │ Status   │ Expires    │
├──────────────┼────────────┼────────────────┼────────────────────────┼──────────┼────────────┤
│ mapping_def  │ 77777777   │ 0.0.0.0:9999   │ tcp://localhost:3306   │ Active   │ 23h 15m    │
│ mapping_ghi  │ 99999999   │ 0.0.0.0:5432   │ tcp://localhost:5432   │ Active   │ 5d 2h      │
└──────────────┴────────────┴────────────────┴────────────────────────┴──────────┴────────────┘

Total: 2 inbound mappings
⚠️  These clients are accessing your services
```

---

### 5. show-mapping - 查看映射详情

**语法**：
```bash
tunnox> show-mapping <mapping-id>
```

**输出示例**：
```bash
tunnox> show-mapping mapping_abc

Mapping Details: mapping_abc
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Basic Info:
  Mapping ID:         mapping_abc
  Type:               Outbound (I'm accessing)
  Status:             Active
  
Connection Info:
  My Listen:          127.0.0.1:3306
  Remote Target:      tcp://192.168.100.10:3306
  Target Client:      88888888
  
Time Info:
  Created:            2025-11-28 12:30:00
  Expires:            2025-12-05 12:30:00 (6d 23h)
  Last Used:          2025-11-28 14:25:30 (2m ago)
  
Statistics:
  Total Connections:  142
  Active Connections: 3
  Bytes Sent:         1.2 GB
  Bytes Received:     850 MB
  Avg Latency:        45ms
  
Connection Code:
  Original Code:      db7-m3x-k9p (used)
  Code ID:            conncode_xyz789

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

---

### 6. status - 查看客户端状态

**语法**：
```bash
tunnox> status [--output json|table]
```

**输出示例**：
```bash
tunnox> status

Tunnox Client Status
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Client Info:
  Client ID:          88888888
  Type:               Registered
  Version:            1.0.0
  
Server Connection:
  Server:             wss://server1.tunnox.io:7000
  Protocol:           WebSocket
  Status:             ✓ Connected
  Connected Since:    2025-11-28 10:00:00 (2h 30m ago)
  Node ID:            node-0001
  IP:                 203.0.113.45
  Latency:            23ms
  
Resources:
  Connection Codes:   3 (2 active, 1 used)
  Outbound Mappings:  2 active
  Inbound Mappings:   1 active
  Active Tunnels:     3
  
Statistics:
  Total Connections:  256
  Bytes Sent:         3.2 GB
  Bytes Received:     1.8 GB
  Uptime:             2h 30m 45s

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

---

### 7. help - 帮助系统

**语法**：
```bash
tunnox> help [command]
```

**输出示例**：
```bash
tunnox> help

Tunnox Client - Available Commands
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Connection Management:
  connect <server>           连接到服务器
  disconnect                 断开连接
  reconnect                  重新连接
  status                     查看客户端状态
  
Connection Code (As TargetClient):
  generate-code              生成连接码
  list-codes                 列出我的连接码
  show-code <code>           查看连接码详情
  revoke-code <code>         撤销连接码
  
Mapping Management:
  use-code <code>            使用连接码创建映射
  list-mappings              列出映射
  show-mapping <id>          查看映射详情
  delete-mapping <id>        删除映射
  mapping-stats <id>         映射统计信息
  
System:
  config show                显示配置
  config set <key> <value>   设置配置
  logs [options]             查看日志
  version                    显示版本
  help [command]             显示帮助
  clear                      清屏
  exit, quit                 退出
  
Type 'help <command>' for detailed information

tunnox> help generate-code

Command: generate-code
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Description:
  生成一个连接码，允许其他客户端建立到本客户端的映射

Usage:
  generate-code \
    --target <protocol://host:port> \
    --expire <duration> \
    --mapping-duration <duration> \
    [--description <text>] \
    [--output json|table]

Parameters:
  --target            目标地址（必填）
                      格式: tcp://192.168.100.10:8888
                      
  --expire            激活有效期（必填）
                      示例: 10m, 1h, 2h
                      
  --mapping-duration  映射有效期（必填）
                      示例: 1d, 7d, 30d
                      
  --description       描述（可选）
  
  --output            输出格式（可选）
                      选项: table, json
                      默认: table

Examples:
  # 生成10分钟有效的连接码，映射7天有效
  tunnox> generate-code \
      --target tcp://localhost:3306 \
      --expire 10m \
      --mapping-duration 7d
  
  # 带描述
  tunnox> generate-code \
      --target tcp://192.168.1.100:8888 \
      --expire 5m \
      --mapping-duration 1d \
      --description "临时API访问"

Security Notes:
  ⚠️  连接码一次性使用，使用后立即失效
  ⚠️  请通过安全渠道分享（企业IM、加密邮件）
  ⚠️  激活期短，降低泄露风险
```

---

## 🎭 CLI交互完整示例

### 示例1: TargetClient生成连接码并监控

```bash
$ tunnox-client
  _____                        
 |_   _|   _ _ __  _ __   _____  __
   | || | | | '_ \| '_ \ / _ \ \/ /
   | || |_| | | | | | | | (_) >  < 
   |_| \__,_|_| |_|_| |_|\___/_/\_\
                                    
Tunnox Client v1.0.0
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Loading config from: ./config.json
✓ Config loaded

Auto-connecting to wss://server1.tunnox.io:7000...
✓ Connected as ClientID: 88888888

Type 'help' for available commands

tunnox> status

Tunnox Client Status
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Client ID:          88888888
Server:             wss://server1.tunnox.io:7000
Status:             ✓ Connected
Connection Codes:   0
Mappings:           0
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

tunnox> generate-code \
    --target tcp://localhost:3306 \
    --expire 10m \
    --mapping-duration 7d \
    --description "本地MySQL数据库"

🔑 Connection Code Generated
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Code:              db7-m3x-k9p
Target:            tcp://localhost:3306
Activation TTL:    10 minutes
Mapping Duration:  7 days
Expires:           2025-11-28 12:40:00
Description:       本地MySQL数据库
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
⚠️  Share this code securely

tunnox> list-codes

My Connection Codes (1):
┌──────────────┬──────────────────────┬────────┬──────────┐
│ Code         │ Target               │ Status │ Expires  │
├──────────────┼──────────────────────┼────────┼──────────┤
│ db7-m3x-k9p  │ tcp://localhost:3306 │ Active │ 9m 30s   │
└──────────────┴──────────────────────┴────────┴──────────┘

... (等待有人使用) ...

tunnox> list-codes

My Connection Codes (1):
┌──────────────┬──────────────────────┬────────┬────────────┐
│ Code         │ Target               │ Status │ Used By    │
├──────────────┼──────────────────────┼────────┼────────────┤
│ db7-m3x-k9p  │ tcp://localhost:3306 │ Used   │ 77777777   │
└──────────────┴──────────────────────┴────────┴────────────┘

tunnox> list-mappings --type inbound

Inbound Mappings (Who's Accessing Me):
┌──────────────┬────────────┬────────────────┬──────────────────────┬──────────┐
│ Mapping      │ Client     │ Their Listen   │ My Target            │ Expires  │
├──────────────┼────────────┼────────────────┼──────────────────────┼──────────┤
│ mapping_abc  │ 77777777   │ 0.0.0.0:9999   │ tcp://localhost:3306 │ 6d 23h   │
└──────────────┴────────────┴────────────────┴──────────────────────┴──────────┘

tunnox> show-mapping mapping_abc

Mapping Details: mapping_abc
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Type:               Inbound (Someone accessing me)
Client:             77777777
Their Listen:       0.0.0.0:9999
My Target:          tcp://localhost:3306
Status:             Active
Created:            2025-11-28 12:30:15
Expires:            2025-12-05 12:30:15 (6d 23h)

Statistics:
  Connections:      12
  Active:           2
  Bytes Sent:       45.2 MB
  Bytes Received:   128.5 MB
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

tunnox> delete-mapping mapping_abc
⚠️  Are you sure you want to delete this mapping? (yes/no): yes
✓ Mapping deleted successfully
   Client 77777777 can no longer access tcp://localhost:3306

tunnox> exit
Goodbye!
```

### 示例2: ListenClient使用连接码

```bash
$ tunnox-client
Tunnox Client v1.0.0
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Loading config from: ~/.tunnox/config.json
✓ Config loaded

Auto-connecting to wss://server1.tunnox.io:7000...
✓ Connected as ClientID: 77777777

tunnox> use-code db7-m3x-k9p --listen 127.0.0.1:3306

🔍 Validating connection code...
✓ Connection code is valid

📋 Connection Code Information:
   Target:            tcp://192.168.100.10:3306
   Mapping Duration:  7 days
   Description:       本地MySQL数据库
   
🔧 Creating tunnel mapping...
✓ Mapping created successfully

📍 Mapping Details:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Mapping ID:        mapping_abc
My Listen:         127.0.0.1:3306
Remote Target:     tcp://192.168.100.10:3306
Target Client:     88888888
Expires:           2025-12-05 12:30:00 (6d 23h 59m)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

💡 You can now connect to:
   $ mysql -h 127.0.0.1 -P 3306 -u user -p

⚠️  Connection code consumed (one-time use)

tunnox> list-mappings

My Outbound Mappings (1):
┌──────────────┬────────────────┬────────────────────────────┬──────────┐
│ Mapping      │ My Listen      │ Remote Target              │ Expires  │
├──────────────┼────────────────┼────────────────────────────┼──────────┤
│ mapping_abc  │ 127.0.0.1:3306 │ tcp://192.168.100.10:3306  │ 6d 23h   │
└──────────────┴────────────────┴────────────────────────────┴──────────┘

# 实际使用映射
$ mysql -h 127.0.0.1 -P 3306 -u root -p
... (MySQL连接成功) ...

tunnox> mapping-stats mapping_abc

Mapping Statistics: mapping_abc
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Connection Statistics:
  Total Connections:      12
  Active Connections:     1
  Failed Connections:     0
  Avg Connection Time:    450ms
  
Traffic Statistics:
  Bytes Sent:             45.2 MB
  Bytes Received:         128.5 MB
  Total:                  173.7 MB
  Avg Transfer Rate:      2.5 MB/s
  
Performance:
  Avg Latency:            45ms
  Max Latency:            120ms
  Packet Loss:            0.01%
  
Time Info:
  First Connection:       2025-11-28 12:31:00
  Last Connection:        2025-11-28 14:25:30 (2m ago)
  Uptime:                 1h 54m
  
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

tunnox> exit
Goodbye!
```

---

## 🔧 命令行参数模式完整设计

### 启动模式判断

```go
// 伪代码
func main() {
    args := os.Args[1:]
    
    if len(args) == 0 {
        // 无参数 → CLI交互模式
        runCLIMode()
    } else if hasServiceCommand(args) {
        // 有服务命令 → 守护进程模式（无CLI）
        runDaemonMode(args)
    } else {
        // 一次性命令 → 执行后退出
        runOnceCommand(args)
    }
}

// 服务命令列表（启动后持续运行，无CLI）
serviceCommands := []string{
    "--daemon",
    "--use-code",  // 如果带 --daemon 或 --listen
}
```

---

### 模式A: CLI交互模式（默认）

**触发条件**：
```bash
$ tunnox-client                    # 无参数
$ tunnox-client --config custom.json  # 仅指定配置文件
```

**启动流程**：
```
1. 解析命令行参数
   └─ --config <path>  (可选，指定配置文件路径)

2. 加载配置文件
   ├─ 优先级1: --config 指定的路径
   ├─ 优先级2: ./config.json
   ├─ 优先级3: ~/.tunnox/config.json
   ├─ 优先级4: /etc/tunnox/config.json
   └─ 如果都不存在，使用默认配置

3. 显示欢迎界面

4. 如果 auto_connect=true
   └─ 自动连接服务器

5. 进入CLI循环
   ├─ 显示提示符: tunnox>
   ├─ 读取用户输入
   ├─ 解析命令
   ├─ 执行命令
   ├─ 显示结果
   └─ 循环（直到 exit/quit）

6. 清理退出
```

**无CLI界面的情况** - 从不进入CLI：
- ❌ 所有服务类命令都不会在CLI中持续运行

---

### 模式B: 守护进程模式（服务启动）

**触发条件**：
```bash
$ tunnox-client --daemon [options]
$ tunnox-client --use-code <code> --listen <addr>  # 自动启用守护进程
```

**服务命令**（启动后持续运行）：

#### B1. 纯守护进程（保持连接）
```bash
$ tunnox-client --daemon \
    [--server <url>] \
    [--client-id <id>] \
    [--secret-key <key>] \
    [--log-file <path>] \
    [--pid-file <path>]

# 示例
$ tunnox-client --daemon \
    --server wss://server1.tunnox.io:7000 \
    --client-id 88888888 \
    --secret-key your-key \
    --log-file /var/log/tunnox-client.log \
    --pid-file /var/run/tunnox-client.pid

[2025-11-28 12:30:00] INFO: Tunnox Client starting...
[2025-11-28 12:30:01] INFO: Connected to server (ClientID: 88888888)
[2025-11-28 12:30:01] INFO: Daemon started (PID: 12345)
[2025-11-28 12:30:01] INFO: Press Ctrl+C to stop

# 进程持续运行，直到手动停止
```

#### B2. 守护进程 + 映射（使用连接码）
```bash
$ tunnox-client \
    --use-code <code> \
    --listen <address> \
    [--daemon] \
    [--server <url>] \
    [--log-file <path>]

# 示例
$ tunnox-client \
    --use-code db7-m3x-k9p \
    --listen 0.0.0.0:9999 \
    --daemon \
    --log-file /var/log/tunnox-mapping.log

[2025-11-28 12:30:00] INFO: Tunnox Client starting...
[2025-11-28 12:30:01] INFO: Connecting to wss://server1.tunnox.io:7000
[2025-11-28 12:30:01] INFO: Connected (ClientID: 77777777)
[2025-11-28 12:30:02] INFO: Activating connection code: db7-m3x-k9p
[2025-11-28 12:30:02] INFO: Target: tcp://192.168.100.10:3306
[2025-11-28 12:30:02] INFO: Mapping created: mapping_abc
[2025-11-28 12:30:02] INFO: Listening on 0.0.0.0:9999
[2025-11-28 12:30:02] INFO: Daemon started (PID: 12346)

# 进程持续运行，映射保持活跃
```

**守护进程特点**：
- ❌ 无CLI界面
- ✅ 后台持续运行
- ✅ 日志输出到文件
- ✅ 创建PID文件
- ✅ 信号处理（SIGTERM优雅退出）
- ✅ 自动重连（如果配置）

---

### 模式C: 一次性命令（执行后退出）

**触发条件**：
```bash
$ tunnox-client <command> [options]
```

**支持的一次性命令**：

#### C1. 生成连接码
```bash
$ tunnox-client generate-code \
    --target tcp://localhost:3306 \
    --expire 10m \
    --mapping-duration 7d

🔑 Connection Code: db7-m3x-k9p
Target: tcp://localhost:3306
Activation Period: 10 minutes
Mapping Duration: 7 days
⚠️  One-time use only

# JSON输出（便于脚本解析）
$ tunnox-client generate-code \
    --target tcp://localhost:3306 \
    --expire 10m \
    --mapping-duration 7d \
    --output json

{"success":true,"data":{"code":"db7-m3x-k9p","target":"tcp://localhost:3306",...}}
```

#### C2. 列出连接码
```bash
$ tunnox-client list-codes

My Connection Codes (3):
  db7-m3x-k9p: tcp://localhost:3306 (Active, expires in 9m)
  web-5a2-n8k: tcp://localhost:8080 (Used by 77777777)
  ssh-7x9-p4m: tcp://localhost:22 (Expired)

$ tunnox-client list-codes --output json
{"success":true,"data":[{"code":"db7-m3x-k9p",...},...]}
```

#### C3. 列出映射
```bash
$ tunnox-client list-mappings

Outbound Mappings (2):
  mapping_abc: 127.0.0.1:3306 → tcp://192.168.100.10:3306 (6d 23h)
  mapping_xyz: 0.0.0.0:8080 → tcp://10.0.0.5:8080 (2d 5h)

$ tunnox-client list-mappings --type inbound

Inbound Mappings (1):
  mapping_def: Client 77777777 accessing tcp://localhost:3306 (23h)
```

#### C4. 查看状态
```bash
$ tunnox-client status

Client: 88888888
Server: wss://server1.tunnox.io:7000
Status: Connected
Uptime: 2h 30m
Mappings: 2 outbound, 1 inbound
```

#### C5. 撤销连接码/映射
```bash
$ tunnox-client revoke-code db7-m3x-k9p
✓ Connection code revoked

$ tunnox-client delete-mapping mapping_abc
✓ Mapping deleted
```

#### C6. 停止守护进程
```bash
$ tunnox-client stop

Stopping Tunnox Client...
✓ Client stopped (PID: 12345)

# 或指定PID文件
$ tunnox-client stop --pid-file /var/run/tunnox-client.pid
```

**一次性命令特点**：
- ❌ 无CLI界面
- ✅ 执行后立即退出
- ✅ 返回状态码（0=成功，非0=失败）
- ✅ 支持JSON输出（便于脚本解析）
- ✅ 适合自动化和脚本集成

---

## 🏗️ 启动模式完整对比

| 特性 | CLI交互模式 | 守护进程模式 | 一次性命令 |
|------|------------|------------|-----------|
| **启动** | `tunnox-client` | `tunnox-client --daemon` | `tunnox-client <cmd>` |
| **CLI界面** | ✅ 有 | ❌ 无 | ❌ 无 |
| **运行方式** | 前台交互 | 后台持续运行 | 执行后退出 |
| **config.json** | ✅ 自动加载 | ✅ 自动加载 | ✅ 自动加载 |
| **输出** | 彩色、表格、友好 | 日志文件 | 简洁或JSON |
| **适用场景** | 日常开发、调试 | 生产环境、持久化服务 | 脚本、自动化 |
| **Tab补全** | ✅ | ❌ | ❌ |
| **历史记录** | ✅ (~/.tunnox/history) | ❌ | ❌ |
| **PID文件** | ❌ | ✅ | ❌ |
| **日志文件** | ⚠️ 可选 | ✅ 必须 | ⚠️ 可选 |
| **错误处理** | 友好提示，继续 | 记录日志，继续 | 返回错误码，退出 |

---

## 📦 SystemD集成（生产环境）

### tunnox-client.service

```ini
[Unit]
Description=Tunnox Client Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=tunnox
Group=tunnox
WorkingDirectory=/opt/tunnox
ExecStart=/usr/local/bin/tunnox-client --daemon \
    --config /etc/tunnox/config.json \
    --log-file /var/log/tunnox/client.log \
    --pid-file /var/run/tunnox-client.pid
ExecStop=/usr/local/bin/tunnox-client stop --pid-file /var/run/tunnox-client.pid
Restart=always
RestartSec=10s
StandardOutput=append:/var/log/tunnox/client.log
StandardError=append:/var/log/tunnox/client-error.log

[Install]
WantedBy=multi-user.target
```

**使用**：
```bash
$ sudo systemctl start tunnox-client
$ sudo systemctl enable tunnox-client
$ sudo systemctl status tunnox-client
```

---

## 🔄 完整使用流程

### 场景：TargetClient临时授权外部访问内网数据库

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. TargetClient生成连接码                                        │
└─────────────────────────────────────────────────────────────────┘

TargetClient (88888888):
  内网数据库: 192.168.100.10:3306
  
  tunnox> generate-code \
      --target tcp://192.168.100.10:3306 \
      --expire 10m \
      --mapping-duration 1d
  
  → 生成连接码: db7-a8x-m2n
  → 通过安全渠道分享给需要访问的人（如企业IM、邮件）

┌─────────────────────────────────────────────────────────────────┐
│ 2. ListenClient收到连接码并激活                                  │
└─────────────────────────────────────────────────────────────────┘

ListenClient (77777777):
  收到连接码: db7-a8x-m2n
  
  tunnox> use-code db7-a8x-m2n --listen 127.0.0.1:3306
  
  → 服务器验证连接码
  → 创建映射: mapping_abc123
  → 连接码失效（已使用）
  → 本地监听: 127.0.0.1:3306
  → 转发到: tcp://192.168.100.10:3306 (TargetClient内网)

┌─────────────────────────────────────────────────────────────────┐
│ 3. ListenClient使用映射访问数据库                                │
└─────────────────────────────────────────────────────────────────┘

ListenClient本地:
  $ mysql -h 127.0.0.1 -P 3306 -u user -p
  
  流量路径:
  本地MySQL客户端
    → 127.0.0.1:3306 (ListenClient本地监听)
    → Tunnox服务器
    → TargetClient
    → 192.168.100.10:3306 (内网数据库)

┌─────────────────────────────────────────────────────────────────┐
│ 4. TargetClient监控和管理访问                                    │
└─────────────────────────────────────────────────────────────────┘

TargetClient:
  tunnox> list-inbound-mappings
  
  → 看到ListenClient (77777777) 正在访问
  → 监控使用情况：连接次数、流量
  → 必要时撤销: revoke-mapping mapping_abc123

┌─────────────────────────────────────────────────────────────────┐
│ 5. 映射到期或撤销                                                │
└─────────────────────────────────────────────────────────────────┘

1天后:
  → 映射自动过期
  → ListenClient无法再连接
  → TargetClient也可提前撤销
```

---

## 📝 存储键设计

### Redis键前缀

```go
// 连接码存储（Runtime，有TTL）
const (
    // 按Code存储（用于快速激活）
    // tunnox:runtime:conncode:code:{code}
    KeyPrefixConnectionCodeByCode = "tunnox:runtime:conncode:code:"
    
    // 按ID存储（用于管理）
    // tunnox:runtime:conncode:id:{id}
    KeyPrefixConnectionCodeByID = "tunnox:runtime:conncode:id:"
    
    // TargetClient的连接码列表
    // tunnox:index:conncode:target:{target_client_id}
    KeyPrefixConnectionCodeByTarget = "tunnox:index:conncode:target:"
)

// 隧道映射存储（Runtime，有TTL）
const (
    // 按ID存储
    // tunnox:runtime:mapping:id:{id}
    KeyPrefixTunnelMappingByID = "tunnox:runtime:mapping:id:"
    
    // ListenClient的映射列表
    // tunnox:index:mapping:listen:{listen_client_id}
    KeyPrefixTunnelMappingByListen = "tunnox:index:mapping:listen:"
    
    // TargetClient的映射列表（谁在访问我）
    // tunnox:index:mapping:target:{target_client_id}
    KeyPrefixTunnelMappingByTarget = "tunnox:index:mapping:target:"
)
```

---

---

## ⚙️ 配置管理详细设计

### 配置文件位置（优先级）

```
1. 命令行指定: --config /path/to/config.json
2. 当前目录:   ./config.json
3. 用户目录:   ~/.tunnox/config.json
4. 系统目录:   /etc/tunnox/config.json
5. 默认配置:   内置默认值
```

### 完整配置结构

```go
// ClientConfig 客户端配置
type ClientConfig struct {
    // 服务器连接（必填）
    ServerURL  string `json:"server_url"`   // "wss://server1.tunnox.io:7000"
    Protocol   string `json:"protocol"`     // "websocket" | "tcp" | "udp" | "quic"
    
    // 客户端身份（必填）
    ClientID   int64  `json:"client_id"`    // 88888888
    SecretKey  string `json:"secret_key"`   // "your-secret-key"
    
    // 连接选项
    AutoConnect       bool   `json:"auto_connect"`        // 默认 false
    Reconnect         bool   `json:"reconnect"`           // 默认 true
    ReconnectInterval string `json:"reconnect_interval"`  // "5s"
    HeartbeatInterval string `json:"heartbeat_interval"`  // "30s"
    ConnectTimeout    string `json:"connect_timeout"`     // "10s"
    
    // 日志配置
    LogLevel string `json:"log_level"` // "debug"|"info"|"warn"|"error"
    LogFile  string `json:"log_file"`  // "" 表示stdout
    
    // TLS配置
    TLS TLSConfig `json:"tls"`
    
    // 代理配置
    Proxy ProxyConfig `json:"proxy"`
}

type TLSConfig struct {
    Enabled      bool   `json:"enabled"`       // 默认 true
    SkipVerify   bool   `json:"skip_verify"`   // 默认 false（生产环境禁止）
    CACert       string `json:"ca_cert"`       // CA证书路径
    ClientCert   string `json:"client_cert"`   // 客户端证书路径
    ClientKey    string `json:"client_key"`    // 客户端私钥路径
}

type ProxyConfig struct {
    Enabled  bool   `json:"enabled"`  // 默认 false
    Type     string `json:"type"`     // "http" | "socks5"
    Address  string `json:"address"`  // "127.0.0.1:1080"
    Username string `json:"username"` // 可选
    Password string `json:"password"` // 可选
}
```

### 配置覆盖优先级

```
命令行参数 > 环境变量 > config.json > 默认值
```

**示例**：
```bash
# config.json
{
  "server_url": "wss://server1.tunnox.io:7000",
  "client_id": 88888888
}

# 环境变量
export TUNNOX_SERVER_URL="wss://server2.tunnox.io:7000"
export TUNNOX_CLIENT_ID=99999999

# 命令行
$ tunnox-client --server wss://server3.tunnox.io:7000 --client-id 11111111

# 最终生效:
# server_url: wss://server3.tunnox.io:7000 (命令行)
# client_id: 11111111 (命令行)
```

### 运行时配置管理

```bash
# CLI中查看配置
tunnox> config show

Current Configuration:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Server:
  URL:                wss://server1.tunnox.io:7000
  Protocol:           websocket
  
Client:
  ID:                 88888888
  Secret Key:         ****** (hidden)
  
Connection:
  Auto Connect:       true
  Reconnect:          true
  Reconnect Interval: 5s
  Heartbeat:          30s
  
Logging:
  Level:              info
  File:               /var/log/tunnox.log
  
Config File:
  Path:               ~/.tunnox/config.json
  Last Modified:      2025-11-28 10:00:00
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# 运行时修改配置（仅当前会话）
tunnox> config set log_level debug
✓ Log level set to debug

tunnox> config set reconnect false
✓ Auto-reconnect disabled

# 保存配置到文件
tunnox> config save
✓ Configuration saved to ~/.tunnox/config.json
```

---

## 🎯 CLI命令实现路线图

### Phase 1: 核心框架 (4小时)
- [ ] CLI引擎（基于readline/liner）
- [ ] 命令解析器（支持参数、flags）
- [ ] Tab补全引擎
- [ ] 历史记录管理（~/.tunnox/history）
- [ ] 彩色输出（基于termcolor/lipgloss）

### Phase 2: 连接管理命令 (2小时)
- [ ] `connect` 命令
- [ ] `disconnect` 命令
- [ ] `reconnect` 命令
- [ ] `status` 命令

### Phase 3: 连接码管理命令 (4小时)
- [ ] `generate-code` 命令
- [ ] `list-codes` 命令
- [ ] `show-code` 命令
- [ ] `revoke-code` 命令

### Phase 4: 映射管理命令 (4小时)
- [ ] `use-code` 命令
- [ ] `list-mappings` 命令
- [ ] `show-mapping` 命令
- [ ] `delete-mapping` 命令
- [ ] `mapping-stats` 命令

### Phase 5: 系统命令 (2小时)
- [ ] `config show/set/save` 命令
- [ ] `logs` 命令
- [ ] `version` 命令
- [ ] `help` 命令（带详细帮助）

### Phase 6: 守护进程模式 (4小时)
- [ ] 守护进程启动框架
- [ ] PID文件管理
- [ ] 信号处理（SIGTERM/SIGINT优雅退出）
- [ ] 日志文件管理（轮转）
- [ ] `stop` 命令

### Phase 7: 一次性命令模式 (2小时)
- [ ] 命令行模式路由
- [ ] JSON输出支持
- [ ] 错误码返回
- [ ] 脚本友好输出

### Phase 8: E2E测试 (4小时)
- [ ] CLI交互模式测试
- [ ] 守护进程模式测试
- [ ] 一次性命令测试
- [ ] 配置加载优先级测试

**CLI总工作量**: 26小时

---

## 🛠️ 技术实现细节

### CLI框架选择

**推荐方案**: **cobra + viper + liner**

```go
// cmd/client/main.go
package main

import (
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

func main() {
    rootCmd := &cobra.Command{
        Use: "tunnox-client",
        Run: func(cmd *cobra.Command, args []string) {
            // 无子命令 → CLI交互模式
            runCLIMode()
        },
    }
    
    // 守护进程模式
    daemonCmd := &cobra.Command{
        Use: "daemon",
        Run: runDaemonMode,
    }
    rootCmd.AddCommand(daemonCmd)
    
    // 一次性命令
    generateCodeCmd := &cobra.Command{
        Use: "generate-code",
        Run: runGenerateCode,
    }
    rootCmd.AddCommand(generateCodeCmd)
    
    // ... 其他命令
    
    rootCmd.Execute()
}

// CLI交互模式
func runCLIMode() {
    cli := NewInteractiveCLI()
    cli.Run()
}
```

### CLI交互引擎

```go
// internal/client/cli/interactive.go
package cli

import (
    "github.com/peterh/liner"
)

type InteractiveCLI struct {
    liner    *liner.State
    client   *Client
    commands map[string]Command
}

func (cli *InteractiveCLI) Run() {
    defer cli.liner.Close()
    
    // 加载历史记录
    cli.loadHistory()
    
    // 显示欢迎界面
    cli.showWelcome()
    
    // 主循环
    for {
        line, err := cli.liner.Prompt("tunnox> ")
        if err != nil {
            break
        }
        
        // 添加到历史
        cli.liner.AppendHistory(line)
        
        // 解析并执行命令
        if err := cli.executeCommand(line); err != nil {
            if err == ErrExit {
                break
            }
            cli.printError(err)
        }
    }
    
    // 保存历史记录
    cli.saveHistory()
}

// Tab补全
func (cli *InteractiveCLI) setupCompletion() {
    cli.liner.SetCompleter(func(line string) []string {
        // 补全命令名
        if !strings.Contains(line, " ") {
            return cli.completeCommand(line)
        }
        // 补全参数
        return cli.completeArgs(line)
    })
}
```

### 表格输出

```go
// internal/client/cli/formatter.go
package cli

import (
    "github.com/olekukonko/tablewriter"
)

func (cli *InteractiveCLI) printCodesTable(codes []*models.TunnelConnectionCode) {
    table := tablewriter.NewWriter(os.Stdout)
    table.SetHeader([]string{"Code", "Target", "Status", "Expires"})
    table.SetBorder(true)
    table.SetRowLine(true)
    
    for _, code := range codes {
        status := cli.getCodeStatus(code)
        expires := cli.formatExpiry(code)
        table.Append([]string{
            code.Code,
            truncate(code.TargetAddress, 30),
            status,
            expires,
        })
    }
    
    table.Render()
}
```

---

## 🎯 完整实施路线图（更新）

### Backend实施（24小时）

#### Phase 1: 数据模型和Repository (4小时)
- [ ] 重命名模型：`TunnelAuthCode` → `TunnelConnectionCode`
- [ ] 重命名模型：`TunnelAccessPermit` → `TunnelMapping`
- [ ] 删除 `SourceClientID` 绑定字段
- [ ] 强制 `TargetAddress` 必填
- [ ] 更新Repository：`ConnectionCodeRepository`, `TunnelMappingRepository`
- [ ] 单元测试（覆盖率 ≥85%）

#### Phase 2: 连接码生成器 (✅ 已完成)
- [x] `ConnectionCodeGenerator`（复用AuthCodeGenerator）
- [x] 格式：`abc-def-123`
- [x] 单元测试（覆盖率 100%）

#### Phase 3: ConnectionCodeService (6小时)
- [ ] 重构 `AuthCodeService` → `ConnectionCodeService`
- [ ] 简化激活逻辑（去除ClientID绑定验证）
- [ ] 强制验证 `TargetAddress`
- [ ] 实现一次性使用原子性检查
- [ ] 单元测试（覆盖率 ≥85%）

#### Phase 4: 集成到隧道验证 (4小时)
- [ ] 扩展 `TunnelOpenRequest`：添加 `MappingID`
- [ ] 修改 `HandleTunnelOpen`：优先验证MappingID
- [ ] 兼容SecretKey（向后兼容）
- [ ] 集成测试

#### Phase 5: API接口 (4小时)
- [ ] 重构 `handlers_authcode.go` → `handlers_connection_code.go`
- [ ] 更新路由：`/api/auth-codes` → `/api/connection-codes`
- [ ] API测试

#### Phase 6: E2E测试 (2小时)
- [ ] 生成连接码 → 激活 → 流量转发
- [ ] 一次性使用验证
- [ ] 并发激活测试

### Frontend实施（CLI + 命令行，26小时）

#### Phase 7: 项目结构 (2小时)
```
cmd/client/
├── main.go                  # 入口，cobra命令树
├── cli/                     # CLI交互模式
│   ├── interactive.go       # 交互引擎
│   ├── commands.go          # 命令实现
│   ├── formatter.go         # 输出格式化
│   ├── completer.go         # Tab补全
│   └── history.go           # 历史记录
├── daemon/                  # 守护进程模式
│   ├── daemon.go            # 守护进程启动
│   ├── pid.go               # PID文件管理
│   └── signals.go           # 信号处理
└── commands/                # 一次性命令
    ├── generate_code.go
    ├── list_codes.go
    ├── use_code.go
    └── ...
```

#### Phase 8: 核心CLI框架 (4小时)
- [ ] 集成 cobra + viper
- [ ] 配置加载（多优先级）
- [ ] 环境变量支持
- [ ] 启动模式判断逻辑

#### Phase 9: CLI交互引擎 (4小时)
- [ ] 基于 liner 的交互循环
- [ ] 命令解析器
- [ ] Tab补全实现
- [ ] 历史记录（~/.tunnox/history）
- [ ] 彩色输出（termcolor）

#### Phase 10: 连接码命令 (4小时)
- [ ] `generate-code` 命令实现
- [ ] `list-codes` 命令实现
- [ ] `show-code` 命令实现
- [ ] `revoke-code` 命令实现

#### Phase 11: 映射命令 (4小时)
- [ ] `use-code` 命令实现
- [ ] `list-mappings` 命令实现
- [ ] `show-mapping` 命令实现
- [ ] `delete-mapping` 命令实现
- [ ] `mapping-stats` 命令实现

#### Phase 12: 系统命令 (2小时)
- [ ] `connect/disconnect/reconnect` 命令
- [ ] `status` 命令
- [ ] `config show/set/save` 命令
- [ ] `version/help` 命令

#### Phase 13: 守护进程模式 (4小时)
- [ ] 守护进程启动框架
- [ ] PID文件管理
- [ ] 信号处理（SIGTERM/SIGINT）
- [ ] 日志轮转
- [ ] 自动重连

#### Phase 14: 输出格式化 (2小时)
- [ ] 表格输出（tablewriter）
- [ ] JSON输出（--output json）
- [ ] 进度条（长时间操作）
- [ ] 错误友好提示

### 总工作量估算

| 模块 | 工作量 | 说明 |
|------|--------|------|
| Backend（连接码系统） | 24小时 | 数据模型、服务、API、测试 |
| Frontend（CLI） | 26小时 | 交互引擎、命令实现、守护进程 |
| **总计** | **50小时** | 约 6-7 个工作日 |

---

## 📊 错误处理和用户体验

### CLI错误提示（友好）

```bash
# 连接失败
tunnox> connect wss://invalid-server.com
❌ Connection failed: dial tcp: lookup invalid-server.com: no such host
💡 Tip: Check your server URL and network connection

# 连接码无效
tunnox> use-code invalid-code --listen 0.0.0.0:9999
❌ Error: Connection code not found
   Code: invalid-code
💡 Tip: Check if the code is correct (format: xxx-yyy-zzz)

# 连接码已使用
tunnox> use-code abc-def-123 --listen 0.0.0.0:9999
❌ Error: Connection code already used
   Code:     abc-def-123
   Used by:  ClientID 77777777
   Used at:  2025-11-28 12:00:00
   Mapping:  mapping_xyz789
💡 Tip: Request a new connection code from the target client

# 端口已被占用
tunnox> use-code xyz-789-abc --listen 0.0.0.0:3306
❌ Error: Address already in use
   Port: 3306
💡 Tip: Choose a different port or stop the service using port 3306

# 权限不足
tunnox> use-code mno-pqr-stu --listen 0.0.0.0:80
❌ Error: Permission denied
   Port: 80 (requires root privileges)
💡 Tip: Use a port > 1024 or run with sudo
```

### 命令行错误码（脚本友好）

```bash
# 成功
$ tunnox-client generate-code --target tcp://localhost:3306 --expire 10m --mapping-duration 7d
Connection Code: abc-def-123
$ echo $?
0

# 参数错误
$ tunnox-client generate-code --target invalid
Error: Invalid target address format
$ echo $?
1

# 服务错误
$ tunnox-client use-code nonexistent --listen 0.0.0.0:9999
Error: Connection code not found
$ echo $?
2

# 网络错误
$ tunnox-client --server wss://unreachable.com status
Error: Connection timeout
$ echo $?
3
```

**错误码定义**：
```go
const (
    ExitSuccess         = 0  // 成功
    ExitInvalidArgs     = 1  // 参数错误
    ExitNotFound        = 2  // 资源不存在
    ExitNetworkError    = 3  // 网络错误
    ExitPermissionDenied = 4  // 权限不足
    ExitAlreadyExists   = 5  // 资源已存在
    ExitInternalError   = 99 // 内部错误
)
```

---

## 🔄 与现有系统的兼容性

### 向后兼容SecretKey
- ✅ `HandleTunnelOpen` 优先使用新的映射验证
- ✅ 如果没有MappingID，回退到SecretKey验证
- ✅ 保持现有API端点工作

### 数据迁移
- ✅ 新系统独立存储（`conncode:*`, `mapping:*`）
- ✅ 旧系统继续使用（`port_mapping:*`）
- ✅ 逐步迁移或并行运行

---

## 📚 参考资料

- [原AuthCode设计](./TUNNEL_TWO_STAGE_AUTH_DESIGN.md) - 保留作为对比
- [实施路线图](./IMPLEMENTATION_ROADMAP.md)
- [安全加固计划](./CONNECTION_SECURITY_HARDENING.md)

---

## 📝 设计总结

### 核心创新点

1. **连接码（ConnectionCode）** - 全局唯一、一次性使用、短期有效
   - ✅ 无需预先绑定ClientID
   - ✅ 强制包含目标地址
   - ✅ 好记格式（abc-def-123）

2. **隧道映射（TunnelMapping）** - 长期有效、绑定ListenClient、可撤销
   - ✅ 从连接码激活创建
   - ✅ 独立的有效期（7天）
   - ✅ 完整的使用统计

3. **CLI设计** - 三种运行模式
   - ✅ **CLI交互模式**（默认）- 日常开发，交互友好
   - ✅ **守护进程模式** - 生产环境，持久化运行
   - ✅ **一次性命令** - 脚本集成，自动化友好

4. **配置管理** - 最小化、优先级清晰
   - ✅ config.json 仅包含基础连接信息
   - ✅ 命令行参数 > 环境变量 > 配置文件 > 默认值

### 关键设计决策

| 决策 | 理由 | 优势 |
|------|------|------|
| 去除ClientID绑定 | 连接码全局唯一即可 | 更灵活、更简单 |
| 强制TargetAddress | 明确访问目标 | 更安全、更直观 |
| 一次性使用 | 降低泄露风险 | 更安全 |
| 两阶段授权 | 区分激活和使用 | 更灵活 |
| CLI作为客户端界面 | 不是独立工具 | 更统一 |
| 三种运行模式 | 覆盖不同场景 | 更通用 |

### 安全保障

| 层面 | 措施 |
|------|------|
| 连接码泄露 | 短期有效（10分钟）、一次性使用、可撤销 |
| 映射劫持 | 绑定ListenClient、使用统计、可撤销 |
| 暴力破解 | 高熵值（4.6×10^13）、失败限制、IP黑名单 |
| 审计追踪 | 完整日志、使用统计、时间戳 |

### 用户体验

| 场景 | 体验 |
|------|------|
| 日常开发 | CLI交互模式，Tab补全、历史记录、彩色输出 |
| 生产部署 | 守护进程模式，SystemD集成、日志轮转 |
| 脚本自动化 | 一次性命令，JSON输出、错误码返回 |
| 错误处理 | 友好提示、具体原因、操作建议 |

### 技术栈

| 组件 | 技术选择 | 原因 |
|------|---------|------|
| 命令框架 | cobra | 强大、生态完善 |
| 配置管理 | viper | 多格式、优先级清晰 |
| CLI引擎 | liner | 轻量、功能完备 |
| 表格输出 | tablewriter | 美观、易用 |
| 彩色输出 | termcolor/lipgloss | 视觉友好 |

### 实施优先级

**P0（核心，Week 1）**:
- 数据模型重构（ConnectionCode + TunnelMapping）
- ConnectionCodeService业务逻辑
- 隧道验证集成

**P1（基础CLI，Week 2）**:
- CLI框架搭建
- 核心命令（generate-code, use-code, list-*)
- 配置管理

**P2（完整功能，Week 3）**:
- 守护进程模式
- 一次性命令模式
- 表格输出、JSON输出

**P3（优化，Week 4）**:
- Tab补全、历史记录
- 错误提示优化
- E2E测试

### 向后兼容

- ✅ 保留SecretKey验证（API调用）
- ✅ 新旧系统并行运行
- ✅ 独立存储键（不冲突）
- ✅ 3个月弃用期

---

## 🎓 使用最佳实践

### TargetClient（生成连接码）

1. **选择合适的激活期**
   - ✅ 临时分享：5-10分钟
   - ✅ 内部团队：30-60分钟
   - ❌ 避免过长（降低安全风险）

2. **选择合适的映射期**
   - ✅ 临时访问：1-3天
   - ✅ 项目合作：7-14天
   - ✅ 长期访问：30天
   - ❌ 避免无限期

3. **监控访问**
   - ✅ 定期查看 `list-mappings --type inbound`
   - ✅ 检查异常流量
   - ✅ 及时撤销不需要的映射

### ListenClient（使用连接码）

1. **及时激活**
   - ✅ 收到连接码后尽快激活（有效期短）
   - ❌ 避免等到最后一分钟

2. **选择合适的监听地址**
   - ✅ 仅本地：`127.0.0.1:port`
   - ✅ 局域网：`0.0.0.0:port`
   - ❌ 避免暴露到公网（安全风险）

3. **监控使用**
   - ✅ 使用 `mapping-stats` 查看统计
   - ✅ 注意有效期即将到期
   - ✅ 不需要时及时删除映射

### 生产环境部署

1. **守护进程模式**
   - ✅ 使用SystemD管理
   - ✅ 配置日志轮转
   - ✅ 启用自动重连

2. **配置管理**
   - ✅ 使用 `/etc/tunnox/config.json`
   - ✅ 通过环境变量传递敏感信息
   - ❌ 避免在配置文件中明文存储密钥

3. **监控告警**
   - ✅ 监控进程状态
   - ✅ 监控映射数量
   - ✅ 异常流量告警

---

## 📚 参考链接

- [设计变更日志](./DESIGN_CHANGELOG.md)
- [实施路线图](./IMPLEMENTATION_ROADMAP.md)
- [安全加固计划](./CONNECTION_SECURITY_HARDENING.md)
- [旧设计文档（已弃用）](./TUNNEL_TWO_STAGE_AUTH_DESIGN_DEPRECATED.md)

---

**文档版本**: v3.0（连接码 + CLI + 命令行完整设计）
**最后更新**: 2025-11-28
**状态**: ✅ 设计完成，已细化所有部分，待实施
**页数**: 600+ 行，涵盖所有实施细节

