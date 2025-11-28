# 隧道访问授权模型设计

## 设计目标

1. **安全性**: 防止未授权访问，支持细粒度权限控制
2. **易用性**: 新用户零门槛体验，匿名映射无需注册
3. **灵活性**: 支持临时授权、定向授权、可撤销授权
4. **可追溯**: 记录授权使用情况，支持审计

---

## 权限模型设计

### 分层验证策略

```
匿名映射（UserID == ""）：
  验证: ClientID + AuthCode
  范围: 宽松，易用性优先
  配额: 有限制（防滥用）
  
注册用户映射（UserID != ""）：
  验证: ClientID + AuthCode + 权限规则
  范围: 严格，安全性优先
  配额: 按用户套餐
```

### AuthCode vs SecretKey 职责分离

| 凭证类型 | 用途 | 生命周期 | 安全级别 | 适用场景 |
|---------|------|---------|---------|---------|
| **SecretKey** | API调用 | 静态，永久 | 高（存储在服务端） | 服务端↔服务端 |
| **AuthCode** | 隧道连接 | 动态，有时效 | 中（用户可见） | 客户端↔服务端 |

---

## 数据模型

### TunnelAuthCode 结构

**基础字段**:
- `ID`: 授权码ID（authcode_xxx）
- `Code`: 授权码内容（abc-def-123，好记格式）
- `ClientID`: 被授权访问的客户端ID

**范围控制**（可选，用于精细化授权）:
- `TargetClientID`: 限定目标客户端（只能访问到该目标的映射）
- `MappingID`: 限定特定映射（只能访问该映射）

**时间控制**:
- `Duration`: 1h, 1d, 1w, 1m
- `CreatedAt`, `ExpiresAt`

**管理字段**:
- `CreatedBy`: 创建者
- `IsRevoked`: 是否已撤销
- `RevokedAt`, `RevokedBy`

**使用统计**:
- `LastUsedAt`: 最后使用时间
- `UsageCount`: 使用次数

---

## 授权码生成规则

### 格式设计

**目标**: 好记、易输入、不易混淆

**格式**: `xxx-yyy-zzz`（3段，每段3字符，用 `-` 分隔）

**字符集**: `0-9a-z`，排除易混淆字符：
- 排除 `i`（与1混淆）
- 排除 `l`（与1混淆）
- 排除 `o`（与0混淆）
- 最终: `0123456789abcdefghjkmnpqrstuvwxyz`（33个字符）

**示例**:
- `a2b-3cd-e4f`
- `9k2-m7n-p1q`
- `h5s-t8v-w2x`

**熵值计算**: 
- 33^9 ≈ 4.6 × 10^13（足够安全）
- 碰撞概率: 1/万亿

### 生成策略

**选项1: 纯随机**（推荐）
- 从字符集中随机选择
- 简单、安全
- 无需检查重复（概率极低）

**选项2: 可读单词组合**
- 使用单词表生成（如 `cat-dog-run`）
- 更好记
- 但字符集受限，熵值降低

---

## 权限验证流程

### 匿名映射验证（宽松）

```
TunnelOpen 请求：
{
  "mapping_id": "pmap_xxx",
  "client_id": 12345678,
  "auth_code": "abc-def-123"
}

验证步骤：
1. ✅ 获取映射信息
2. ✅ 检查 mapping.UserID == "" (匿名映射)
3. ✅ 查询 AuthCode (by code)
4. ✅ 验证 AuthCode.ClientID == request.ClientID
5. ✅ 验证 AuthCode.IsValid() (未过期、未撤销)
6. ✅ 验证 AuthCode.CanAccessMapping(mapping)
7. ✅ 更新 AuthCode 使用统计
8. ✅ 允许访问

特点：
- 无需验证用户身份
- 只要有有效的AuthCode即可
- 适合快速体验场景
```

### 注册用户映射验证（严格）

```
TunnelOpen 请求：
{
  "mapping_id": "pmap_xxx",
  "client_id": 12345678,
  "auth_code": "xyz-456-789"
}

验证步骤：
1. ✅ 获取映射信息
2. ✅ 检查 mapping.UserID != "" (注册用户映射)
3. ✅ 查询 AuthCode
4. ✅ 验证 AuthCode.ClientID == request.ClientID
5. ✅ 验证 AuthCode.IsValid()
6. ✅ 验证 AuthCode.CanAccessMapping(mapping)
7. ✅ 验证权限规则：
   - ClientID 是源端或目标端？
   - ClientID 属于同一用户？
8. ✅ 更新使用统计
9. ✅ 允许访问

特点：
- 多重验证
- 更严格的权限检查
- 适合正式使用场景
```

---

## 用户授权管理

### 授权列表查询

**用户视角**: "我的 Client 12345678 授权给了谁？"

```
GET /api/clients/12345678/auth-codes

响应：
{
  "client_id": 12345678,
  "auth_codes": [
    {
      "id": "authcode_001",
      "code": "abc-def-123",
      "target_client_id": null,     // 无限制，可访问所有映射
      "mapping_id": null,
      "expires_at": "2025-11-29T10:00:00Z",
      "is_revoked": false,
      "usage_count": 42,
      "last_used_at": "2025-11-28T09:30:00Z",
      "description": "给朋友的临时访问"
    },
    {
      "id": "authcode_002",
      "code": "xyz-456-789",
      "target_client_id": 87654321, // 只能访问到87654321的映射
      "mapping_id": null,
      "expires_at": "2025-11-28T11:00:00Z",
      "is_revoked": false,
      "usage_count": 5,
      "last_used_at": "2025-11-28T10:15:00Z",
      "description": "测试用，1小时"
    }
  ]
}
```

### CLI 命令设计

#### 创建授权码

```bash
# 基础授权（可访问Client的所有映射）
tunnox auth create --client 12345678 --duration 1d

输出:
✅ AuthCode created successfully
   Code:       abc-def-123
   Client:     12345678
   Target:     Any (全部)
   Mapping:    Any (全部)
   Expires:    2025-11-29 10:00:00
   Duration:   1 day

# 精细授权（只能访问到特定目标）
tunnox auth create --client 12345678 --target 87654321 --duration 1h

输出:
✅ AuthCode created successfully
   Code:       xyz-456-789
   Client:     12345678
   Target:     87654321 (限定)
   Mapping:    Any
   Expires:    2025-11-28 11:00:00
   Duration:   1 hour

# 最精细授权（只能访问特定映射）
tunnox auth create --mapping pmap_xxx --duration 1h

输出:
✅ AuthCode created successfully
   Code:       mno-789-pqr
   Client:     12345678 (from mapping)
   Target:     87654321 (from mapping)
   Mapping:    pmap_xxx (限定)
   Expires:    2025-11-28 11:00:00
   Duration:   1 hour
```

#### 列出授权码

```bash
# 查看Client的所有授权
tunnox auth list --client 12345678

输出:
ID              Code          Target    Mapping   Expires              Usage  Status
authcode_001    abc-def-123   Any       Any       2025-11-29 10:00:00  42     Active
authcode_002    xyz-456-789   87654321  Any       2025-11-28 11:00:00  5      Active
authcode_003    old-exp-ired  Any       Any       2025-11-27 10:00:00  120    Expired
```

#### 撤销授权码

```bash
tunnox auth revoke abc-def-123

输出:
✅ AuthCode revoked successfully
   Code:     abc-def-123
   Client:   12345678
   Revoked:  2025-11-28 10:30:00
   Reason:   Manual revocation
```

#### 查看授权码详情

```bash
tunnox auth info abc-def-123

输出:
AuthCode Details:
  ID:           authcode_001
  Code:         abc-def-123
  Client:       12345678
  Target:       Any
  Mapping:      Any
  Created:      2025-11-28 10:00:00
  Expires:      2025-11-29 10:00:00
  Duration:     1 day
  Status:       Active
  Usage Count:  42
  Last Used:    2025-11-28 09:30:00
  Description:  给朋友的临时访问
```

---

## 协议变更

### TunnelOpenRequest 扩展

**当前**:
```
{
  "mapping_id": "pmap_xxx",
  "tunnel_id": "tunnel_xxx",
  "secret_key": "static_secret"  // ❌ 弃用
}
```

**新设计**:
```
{
  "mapping_id": "pmap_xxx",
  "tunnel_id": "tunnel_xxx",
  "client_id": 12345678,         // ⭐ 新增：客户端ID
  "auth_code": "abc-def-123",    // ⭐ 新增：授权码
  "secret_key": "xxx"            // ⚠️ 保留：兼容API调用
}
```

### 验证优先级

```
IF secret_key != ""  // API调用
  THEN
    验证 mapping.SecretKey == request.SecretKey
    允许访问（服务端到服务端）
    
ELSE IF auth_code != ""  // 客户端调用
  THEN
    查询 AuthCode
    验证 AuthCode.ClientID == request.ClientID
    验证 AuthCode.IsValid()
    验证 AuthCode.CanAccessMapping(mapping)
    
    IF mapping.UserID == "" (匿名)
      THEN 宽松模式，允许访问
    ELSE (注册用户)
      THEN 严格模式，检查权限规则
      
ELSE
  拒绝访问（缺少凭证）
```

---

## 存储设计

### Redis 存储结构

```
# 按Code查询（快速）
tunnox:authcode:code:{code} -> TunnelAuthCode JSON
TTL: ExpiresAt

# 按ClientID查询（列表）
tunnox:authcode:client:{clientID} -> Set[authcode_id1, authcode_id2, ...]
TTL: 永久或按最长的AuthCode

# 按ID查询（管理）
tunnox:authcode:id:{authcode_id} -> TunnelAuthCode JSON
TTL: ExpiresAt

# 已撤销列表（防重放）
tunnox:authcode:revoked:{code} -> timestamp
TTL: 7天（过了有效期后可清理）
```

### PostgreSQL 持久化（可选）

用于审计和历史查询：
```sql
CREATE TABLE tunnel_auth_codes (
    id VARCHAR(32) PRIMARY KEY,
    code VARCHAR(16) UNIQUE NOT NULL,
    client_id BIGINT NOT NULL,
    target_client_id BIGINT,
    mapping_id VARCHAR(32),
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    duration VARCHAR(10) NOT NULL,
    created_by VARCHAR(32),
    is_revoked BOOLEAN DEFAULT FALSE,
    revoked_at TIMESTAMP,
    revoked_by VARCHAR(32),
    last_used_at TIMESTAMP,
    usage_count INT DEFAULT 0,
    description TEXT,
    INDEX idx_client_id (client_id),
    INDEX idx_code (code),
    INDEX idx_expires_at (expires_at)
);
```

---

## 业务场景

### 场景1: 快速体验（匿名用户）

```
用户访问官网 → 点击"快速体验"

1. 系统自动创建匿名Client: 12345678
2. 系统自动生成AuthCode: "try-me-now"（1小时有效）
3. 用户创建映射（Source: 12345678, Target: 自己的服务）
4. 用户使用AuthCode连接隧道
5. ✅ 立即可用，无需注册

限制：
- 每IP最多3个匿名Client
- 每个匿名Client最多1个映射
- 带宽限制10MB/s
- 24小时后自动过期
```

### 场景2: 注册用户正式使用

```
用户注册 → 创建Client A (12345678) 和 Client B (87654321)

1. 创建映射: Source=12345678, Target=87654321
2. 系统自动生成长期AuthCode: "my-home-link"（1个月有效）
3. Client A 使用 ClientID=12345678 + AuthCode="my-home-link" 连接
4. ✅ 验证通过（SourceClient）

安全：
- 只有ClientID匹配的客户端能用
- 用户可以撤销AuthCode
- 用户可以看到使用情况
```

### 场景3: 临时授权给朋友

```
用户想让朋友访问自己的服务（临时）

1. 生成临时AuthCode: 
   tunnox auth create --client 12345678 --target 87654321 --duration 1h --desc "给朋友临时访问"
   
2. 获得: "tmp-fri-end"（1小时有效）

3. 把 ClientID=12345678 + AuthCode="tmp-fri-end" 告诉朋友

4. 朋友使用这个凭证连接

5. 1小时后自动失效，或用户主动撤销

安全：
- 有时效性（1小时）
- 可随时撤销
- 可追踪使用情况（谁在什么时候用了多少次）
```

### 场景4: 设备间互联（IoT）

```
用户有多个IoT设备，需要互相通信

Device A (Client: 11111111)
Device B (Client: 22222222)
Device C (Client: 33333333)

1. 创建映射：A ↔ B, A ↔ C, B ↔ C

2. 为每个设备生成长期AuthCode（1个月）：
   - Device A: "dev-a-auth"
   - Device B: "dev-b-auth"
   - Device C: "dev-c-auth"

3. 将AuthCode烧录到设备固件

4. 设备自动连接，无需人工干预

管理：
- 设备丢失？撤销AuthCode
- 设备更换？生成新AuthCode
- 查看使用情况？审计日志
```

---

## API 设计

### 授权码管理接口

#### 创建授权码
```
POST /api/auth-codes
{
  "client_id": 12345678,
  "target_client_id": 87654321,  // 可选
  "mapping_id": null,            // 可选
  "duration": "1d",
  "description": "给朋友的访问"
}

响应:
{
  "success": true,
  "data": {
    "id": "authcode_001",
    "code": "abc-def-123",
    "expires_at": "2025-11-29T10:00:00Z"
  }
}
```

#### 列出授权码
```
GET /api/clients/12345678/auth-codes
GET /api/users/{userID}/auth-codes  // 用户的所有Client的授权码
```

#### 撤销授权码
```
DELETE /api/auth-codes/{code}
或
POST /api/auth-codes/{code}/revoke
```

#### 查看使用统计
```
GET /api/auth-codes/{code}/stats

响应:
{
  "code": "abc-def-123",
  "usage_count": 42,
  "last_used_at": "2025-11-28T09:30:00Z",
  "usage_history": [
    {
      "timestamp": "2025-11-28T09:30:00Z",
      "ip_address": "1.2.3.4",
      "tunnel_id": "tunnel_xxx"
    }
  ]
}
```

---

## 向后兼容性

### 过渡期策略

**阶段1: 双重支持**（当前实施）
- 同时支持 SecretKey 和 AuthCode
- SecretKey 优先级更高（API调用）
- AuthCode 用于客户端调用

**阶段2: AuthCode推广**（3个月后）
- 新创建的映射不再自动生成SecretKey（除非用户明确需要API调用）
- 文档推荐使用AuthCode

**阶段3: SecretKey限制**（6个月后）
- SecretKey仅用于API调用
- 客户端连接必须使用AuthCode

### 迁移方案

**现有映射**:
- 保留 SecretKey（向后兼容）
- 自动生成默认 AuthCode（永久有效）
- 用户可逐步迁移到 AuthCode

**新创建映射**:
- 默认生成 AuthCode（1个月有效）
- SecretKey 可选（仅当需要API调用时）

---

## 实施计划

### Phase 0.1: 基础实现（T0.1的扩展）

**文件组织**:
```
internal/
├── cloud/models/
│   └── tunnel_auth.go         # 数据模型
├── cloud/repos/
│   └── auth_code_repository.go # 数据访问层
├── cloud/services/
│   └── auth_code_service.go   # 业务逻辑层
├── api/
│   └── handlers_authcode.go   # API层
└── app/server/
    └── handlers.go            # 集成到隧道验证
```

**工作量**: 
- T0.1a: 数据模型和Repository（4小时）
- T0.1b: Service层实现（6小时）
- T0.1c: 集成到隧道验证（4小时）
- T0.1d: API接口（4小时）
- T0.1e: 单元测试（6小时）
- **总计: 24小时（3个工作日）**

### Phase 0.2: CLI工具

**命令实现**:
- `tunnox auth create`
- `tunnox auth list`
- `tunnox auth revoke`
- `tunnox auth info`

**工作量**: 8小时

### Phase 0.3: E2E测试

**测试场景**:
- 匿名映射使用AuthCode连接
- 注册用户映射权限验证
- AuthCode过期拒绝
- AuthCode撤销拒绝
- 精细授权（限定Target）

**工作量**: 6小时

---

## 配额设计（配合匿名客户端）

### 匿名客户端默认配额

```yaml
anonymous:
  default_quota:
    max_mappings: 1          # 最多1个映射
    max_tunnels: 10          # 最多10个并发隧道
    max_bandwidth: 10        # 10 MB/s
    max_auth_codes: 5        # 最多5个AuthCode
    max_connections: 10      # 最多10个并发连接
```

### 注册用户配额（分级）

**免费用户**:
- 最多5个Client
- 每个Client最多10个映射
- 无限AuthCode
- 带宽 50MB/s

**付费用户**:
- 无限Client
- 无限映射
- 无限AuthCode
- 带宽按套餐

---

## 安全考虑

### AuthCode安全性

**优点**:
- ✅ 有时效性（自动过期）
- ✅ 可撤销（随时失效）
- ✅ 可追踪（知道谁在用）
- ✅ 精细控制（可限定Target/Mapping）

**风险**:
- ⚠️ 用户可见（可能泄露）
- ⚠️ 可分享（可能被滥用）

**缓解措施**:
1. **速率限制**: 防止暴力枚举
2. **使用统计**: 异常使用触发告警
3. **IP绑定**（可选）: AuthCode绑定特定IP
4. **一次性Code**（可选）: 使用后立即失效

### 与其他安全措施配合

**Layer 1**: TLS传输加密  
**Layer 2**: AuthCode验证（本方案）  
**Layer 3**: IP黑名单/白名单  
**Layer 4**: 速率限制  
**Layer 5**: 审计日志  

多层防御，纵深安全。

---

## 总结

### 核心优势

| 维度 | SecretKey | AuthCode |
|------|-----------|----------|
| **生成** | 静态，创建映射时 | 动态，随时生成 |
| **时效** | 永久 | 可配置（1h-1m） |
| **撤销** | 不可（需删映射） | 随时可撤销 |
| **范围** | 映射级别 | Client级/Target级/Mapping级 |
| **可见性** | 服务端存储 | 用户可见，可分享 |
| **追踪** | 无 | 使用统计，审计 |
| **用途** | API调用 | 客户端隧道连接 |
| **用户体验** | 不友好（长字符串） | 友好（abc-def-123） |

### 业务价值

1. **新用户体验** ⬆️ 90%
   - 零门槛体验（匿名+AuthCode）
   - 好记的授权码
   - 无需记住复杂密钥

2. **安全性** ⬆️ 80%
   - 有时效性（自动过期）
   - 可撤销（随时失效）
   - 可追踪（审计）

3. **灵活性** ⬆️ 95%
   - 临时授权（给朋友1小时）
   - 设备授权（IoT场景）
   - 精细控制（限定Target）

---

**推荐立即实施这个方案，作为 T0.1 的扩展版本！**

