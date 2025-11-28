# 隧道两阶段授权模型设计

## 设计目标

解决授权码分享的安全问题：
- ✅ 授权码可安全分享（即使泄露，10分钟后失效）
- ✅ 激活后获得长期访问权限（10天稳定访问）
- ✅ 授权码一次性使用（防止重复激活）
- ✅ 访问许可可随时撤销（TargetClient控制）

---

## 核心概念

### 两个阶段

| 阶段 | 名称 | 目的 | 生命周期 | 使用次数 |
|------|------|------|---------|---------|
| **阶段1** | 授权码激活 | 安全分享邀请 | 短期（10分钟） | 一次性 |
| **阶段2** | 访问许可使用 | 实际隧道访问 | 长期（10天） | 多次 |

### 两个实体

**TunnelAuthCode** (授权码)
```
作用: 临时邀请码，用于激活访问许可
生成者: TargetClient (被访问的一方)
使用者: SourceClient (访问的一方)
生命周期: 短期（10分钟）
使用次数: 一次性（激活后失效）
存储位置: Redis（TTL自动过期）
```

**TunnelAccessPermit** (访问许可)
```
作用: 长期访问凭证，实际隧道验证使用
生成: 从AuthCode激活后自动创建
有效期: 长期（10天）
使用次数: 多次（直到过期或撤销）
存储位置: Redis（可选持久化到PostgreSQL用于审计）
```

---

## 业务流程

### 完整流程示例

```
场景: TargetClient (88888888) 想让 SourceClient 访问自己的 192.168.8.88:1230/tcp

┌─────────────────────────────────────────────────────────────────┐
│ 阶段1: 生成授权码（TargetClient操作）                           │
└─────────────────────────────────────────────────────────────────┘

1. TargetClient (88888888) 执行命令:
   
   tunnox auth create \
     --target 192.168.8.88:1230/tcp \
     --activation-ttl 10m \
     --access-duration 10d \
     --desc "给朋友的临时访问"

2. 系统生成授权码:
   
   ✅ AuthCode created successfully
      Code:             abc-def-123
      Target:           ClientID 88888888 (192.168.8.88:1230/tcp)
      Activation TTL:   10 minutes (激活有效期)
      Access Duration:  10 days (激活后的访问期)
      Expires:          2025-11-28 10:10:00 (10分钟后)
      
      ⚠️ 请在 10分钟内 告诉对方使用此码激活！

3. TargetClient 告诉 SourceClient:
   
   "嗨，用这个码连接我的服务: abc-def-123"
   （可通过微信、邮件、电话等任意方式分享，即使被截获，10分钟后也失效）


┌─────────────────────────────────────────────────────────────────┐
│ 阶段2: 激活授权码（SourceClient操作）                           │
└─────────────────────────────────────────────────────────────────┘

4. SourceClient (假设 ClientID=77777777) 执行激活:
   
   tunnox auth activate abc-def-123

5. 系统验证并激活:
   
   ✅ 授权码有效性检查
      - 未过期？✅ (还在10分钟内)
      - 未激活？✅ (首次使用)
      - 未撤销？✅
   
   ✅ 创建访问许可 (permit_xxx)
      Source:    ClientID 77777777
      Target:    ClientID 88888888
      Address:   192.168.8.88:1230/tcp
      Expires:   2025-12-08 10:05:00 (10天后)
   
   ✅ 标记授权码为已激活
      IsActivated: true
      ActivatedBy: 77777777
      ActivatedAt: 2025-11-28 10:05:00
      PermitID:    permit_xxx

6. SourceClient 收到确认:
   
   ✅ Activation successful!
      Permit ID:     permit_xxx
      Access Until:  2025-12-08 10:05:00 (10 days)
      
      You can now create tunnels to ClientID 88888888
      Example:
        tunnox tunnel create \
          --source 77777777 \
          --target 88888888 \
          --permit permit_xxx


┌─────────────────────────────────────────────────────────────────┐
│ 阶段3: 使用访问许可（后续隧道连接）                             │
└─────────────────────────────────────────────────────────────────┘

7. SourceClient 创建映射并连接:
   
   # 创建映射
   tunnox mapping create \
     --source 77777777 \
     --target 88888888 \
     --permit permit_xxx

8. 隧道连接时验证:
   
   TunnelOpenRequest {
     "mapping_id": "pmap_xxx",
     "tunnel_id":  "tunnel_xxx",
     "permit_id":  "permit_xxx"  // ⭐ 使用许可ID，而不是授权码
   }

9. 服务端验证访问许可:
   
   ✅ 获取 AccessPermit (permit_xxx)
   ✅ 验证 Permit.IsValid()
      - 未过期？✅ (还在10天内)
      - 未撤销？✅
   ✅ 验证 Permit.CanAccessMapping(mapping)
      - SourceClient 匹配？✅ (77777777)
      - TargetClient 匹配？✅ (88888888)
      - Mapping 匹配？✅
   ✅ 更新使用统计
      UsageCount++
      LastUsedAt = now()
   ✅ 允许连接

10. 隧道建立成功，开始数据传输


┌─────────────────────────────────────────────────────────────────┐
│ 后续管理                                                         │
└─────────────────────────────────────────────────────────────────┘

TargetClient 可以随时撤销访问许可:
  
  tunnox auth revoke-permit permit_xxx
  
  ✅ Permit revoked successfully
     All active tunnels using this permit will be closed.

TargetClient 可以查看谁在访问:
  
  tunnox auth list-permits --target 88888888
  
  Permit ID     Source      Address              Expires              Usage  Status
  permit_xxx    77777777    192.168.8.88:1230    2025-12-08 10:05:00  142    Active
  permit_yyy    66666666    192.168.8.88:1230    2025-12-05 08:30:00  28     Active
```

---

## 数据模型详解

### TunnelAuthCode 字段说明

```go
type TunnelAuthCode struct {
    // 基础信息
    ID             string        // authcode_xxx
    Code           string        // abc-def-123 (好记格式)
    TargetClientID int64         // ⭐ 88888888 (生成授权码的客户端)
    
    // 访问范围（可选）
    TargetAddress  string        // "192.168.8.88:1230/tcp" (限定访问地址)
    MappingID      *string       // nil (不限定特定映射)
    SourceClientID *int64        // nil (任何人都能激活) 或 77777777 (只有特定Client能激活)
    
    // 两阶段时效 ⭐⭐⭐
    ActivationTTL  time.Duration // 10 * time.Minute (激活有效期)
    AccessDuration time.Duration // 10 * 24 * time.Hour (激活后的访问期)
    
    // 时间戳
    CreatedAt      time.Time     // 2025-11-28 10:00:00
    ExpiresAt      time.Time     // 2025-11-28 10:10:00 (CreatedAt + ActivationTTL)
    
    // 激活状态
    IsActivated    bool          // false → true (激活后)
    ActivatedAt    *time.Time    // nil → 2025-11-28 10:05:00
    ActivatedBy    *int64        // nil → 77777777
    PermitID       *string       // nil → "permit_xxx"
    
    // 管理
    CreatedBy      string        // "user_abc" 或 "88888888"
    IsRevoked      bool          // false (可撤销未激活的码)
    
    // 元数据
    Description    string        // "给朋友的临时访问"
}
```

### TunnelAccessPermit 字段说明

```go
type TunnelAccessPermit struct {
    // 基础信息
    ID             string        // permit_xxx
    AuthCodeID     string        // authcode_xxx (来源)
    SourceClientID int64         // ⭐ 77777777 (访问者)
    TargetClientID int64         // ⭐ 88888888 (被访问者)
    
    // 访问范围
    TargetAddress  string        // "192.168.8.88:1230/tcp"
    MappingID      *string       // nil (不限定) 或 "pmap_xxx" (限定)
    
    // 时间控制
    CreatedAt      time.Time     // 2025-11-28 10:05:00 (激活时间)
    ExpiresAt      time.Time     // 2025-12-08 10:05:00 (CreatedAt + AccessDuration)
    AccessDuration time.Duration // 10 * 24 * time.Hour
    
    // 管理
    IsRevoked      bool          // false → true (TargetClient可撤销)
    RevokedAt      *time.Time    // nil → 撤销时间
    RevokedBy      string        // nil → "88888888" 或 "user_abc"
    
    // 使用统计
    LastUsedAt     *time.Time    // 最后一次隧道连接时间
    UsageCount     int64         // 142 (使用次数)
    
    // 元数据
    Description    string        // 从 AuthCode 继承
}
```

---

## 业务方法

### TunnelAuthCode 方法

```go
// IsValid 检查授权码是否可用于激活
func (a *TunnelAuthCode) IsValid() bool {
    // 已激活？不能再次使用
    if a.IsActivated { return false }
    // 已撤销？无效
    if a.IsRevoked { return false }
    // 已过期？无效
    if time.Now().After(a.ExpiresAt) { return false }
    return true
}

// CanActivate 检查指定SourceClient是否可以激活
func (a *TunnelAuthCode) CanActivate(sourceClientID int64) bool {
    if !a.IsValid() { return false }
    
    // 如果限定了SourceClientID，必须匹配
    if a.SourceClientID != nil && *a.SourceClientID != sourceClientID {
        return false
    }
    return true
}
```

### TunnelAccessPermit 方法

```go
// IsValid 检查访问许可是否有效
func (p *TunnelAccessPermit) IsValid() bool {
    // 已撤销？无效
    if p.IsRevoked { return false }
    // 已过期？无效
    if time.Now().After(p.ExpiresAt) { return false }
    return true
}

// CanAccessMapping 检查许可是否允许访问指定映射
func (p *TunnelAccessPermit) CanAccessMapping(mapping *PortMapping) bool {
    if !p.IsValid() { return false }
    
    // 验证SourceClient和TargetClient是否匹配映射
    if mapping.SourceClientID != p.SourceClientID && mapping.TargetClientID != p.SourceClientID {
        return false
    }
    if mapping.SourceClientID != p.TargetClientID && mapping.TargetClientID != p.TargetClientID {
        return false
    }
    
    // 如果限定了MappingID，必须匹配
    if p.MappingID != nil && mapping.ID != *p.MappingID {
        return false
    }
    
    return true
}
```

---

## 存储设计

### Redis 存储结构

#### AuthCode 存储

```
# 按Code查询（快速激活）
tunnox:authcode:code:{code} -> TunnelAuthCode JSON
TTL: ActivationTTL (10分钟后自动删除)

# 按TargetClientID查询（列表）
tunnox:authcode:target:{targetClientID} -> Set[authcode_id1, authcode_id2, ...]
TTL: 永久（需手动清理已激活/过期的）

# 按ID查询（管理）
tunnox:authcode:id:{authcode_id} -> TunnelAuthCode JSON
TTL: ActivationTTL

示例:
tunnox:authcode:code:abc-def-123 -> {
  "id": "authcode_001",
  "code": "abc-def-123",
  "target_client_id": 88888888,
  "target_address": "192.168.8.88:1230/tcp",
  "activation_ttl": "10m",
  "access_duration": "240h",
  "created_at": "2025-11-28T10:00:00Z",
  "expires_at": "2025-11-28T10:10:00Z",
  "is_activated": false
}
TTL: 600 (10分钟)
```

#### AccessPermit 存储

```
# 按PermitID查询（快速验证）
tunnox:permit:id:{permit_id} -> TunnelAccessPermit JSON
TTL: AccessDuration (10天后自动删除)

# 按SourceClientID查询（我的访问许可列表）
tunnox:permit:source:{sourceClientID} -> Set[permit_id1, permit_id2, ...]
TTL: 永久

# 按TargetClientID查询（谁在访问我）
tunnox:permit:target:{targetClientID} -> Set[permit_id1, permit_id2, ...]
TTL: 永久

# 按MappingID查询（映射对应的许可）
tunnox:permit:mapping:{mappingID} -> Set[permit_id1, permit_id2, ...]
TTL: 永久

示例:
tunnox:permit:id:permit_xxx -> {
  "id": "permit_xxx",
  "auth_code_id": "authcode_001",
  "source_client_id": 77777777,
  "target_client_id": 88888888,
  "target_address": "192.168.8.88:1230/tcp",
  "created_at": "2025-11-28T10:05:00Z",
  "expires_at": "2025-12-08T10:05:00Z",
  "access_duration": "240h",
  "is_revoked": false,
  "usage_count": 142,
  "last_used_at": "2025-12-01T15:30:00Z"
}
TTL: 864000 (10天)
```

---

## CLI 命令设计

### 1. 创建授权码（TargetClient操作）

```bash
# 基础用法
tunnox auth create \
  --activation-ttl 10m \
  --access-duration 10d \
  --desc "给朋友的访问"

# 限定访问地址
tunnox auth create \
  --target-address 192.168.8.88:1230/tcp \
  --activation-ttl 10m \
  --access-duration 10d

# 限定SourceClient（只允许特定Client激活）
tunnox auth create \
  --source 77777777 \
  --activation-ttl 10m \
  --access-duration 10d

# 限定映射（只允许访问特定映射）
tunnox auth create \
  --mapping pmap_xxx \
  --activation-ttl 10m \
  --access-duration 10d

输出:
✅ AuthCode created successfully
   Code:             abc-def-123
   Target:           ClientID 88888888 (自动识别)
   Target Address:   192.168.8.88:1230/tcp
   Activation TTL:   10 minutes
   Access Duration:  10 days
   Expires:          2025-11-28 10:10:00
   
   ⚠️  Share this code within 10 minutes: abc-def-123
```

### 2. 激活授权码（SourceClient操作）

```bash
tunnox auth activate abc-def-123

输出:
✅ Activation successful!
   Permit ID:       permit_xxx
   Source:          ClientID 77777777 (你)
   Target:          ClientID 88888888
   Target Address:  192.168.8.88:1230/tcp
   Access Until:    2025-12-08 10:05:00 (10 days)
   
   You can now create tunnels to this target:
     tunnox mapping create --source 77777777 --target 88888888
```

### 3. 列出授权码（TargetClient查看自己生成的码）

```bash
tunnox auth list-codes --target 88888888

输出:
ID            Code          Activation    Access      Expires              Status
authcode_001  abc-def-123   10m           10d         2025-11-28 10:10:00  Activated
authcode_002  def-456-ghi   10m           10d         2025-11-28 10:15:00  Pending
authcode_003  old-exp-ired  10m           10d         2025-11-28 09:50:00  Expired
```

### 4. 列出访问许可

```bash
# TargetClient 查看谁在访问我
tunnox auth list-permits --target 88888888

输出:
Permit ID     Source      Address              Expires              Usage  Status
permit_xxx    77777777    192.168.8.88:1230    2025-12-08 10:05:00  142    Active
permit_yyy    66666666    192.168.8.88:1230    2025-12-05 08:30:00  28     Active

# SourceClient 查看我的访问许可
tunnox auth list-permits --source 77777777

输出:
Permit ID     Target      Address              Expires              Usage  Status
permit_xxx    88888888    192.168.8.88:1230    2025-12-08 10:05:00  142    Active
permit_zzz    99999999    10.0.0.1:8080        2025-12-10 14:20:00  5      Active
```

### 5. 撤销访问许可（TargetClient操作）

```bash
tunnox auth revoke-permit permit_xxx

输出:
✅ Permit revoked successfully
   Permit ID:  permit_xxx
   Source:     77777777
   Revoked:    2025-11-28 10:30:00
   
   ⚠️  All active tunnels using this permit will be closed.
```

### 6. 查看授权码详情

```bash
tunnox auth info-code abc-def-123

输出:
AuthCode Details:
  ID:               authcode_001
  Code:             abc-def-123
  Target:           ClientID 88888888
  Target Address:   192.168.8.88:1230/tcp
  Activation TTL:   10 minutes
  Access Duration:  10 days
  Created:          2025-11-28 10:00:00
  Expires:          2025-11-28 10:10:00
  Status:           Activated
  Activated By:     77777777
  Activated At:     2025-11-28 10:05:00
  Permit ID:        permit_xxx
  Description:      给朋友的访问
```

---

## 协议变更

### TunnelOpenRequest 扩展

**当前**:
```json
{
  "mapping_id": "pmap_xxx",
  "tunnel_id": "tunnel_xxx",
  "secret_key": "static_secret"
}
```

**新设计**:
```json
{
  "mapping_id": "pmap_xxx",
  "tunnel_id": "tunnel_xxx",
  "permit_id": "permit_xxx",     // ⭐ 使用访问许可ID
  "secret_key": "xxx"            // ⚠️ 保留：兼容API调用
}
```

### 验证优先级

```
IF secret_key != ""  // API调用（服务端到服务端）
  THEN
    验证 mapping.SecretKey == request.SecretKey
    允许访问
    
ELSE IF permit_id != ""  // 客户端调用（使用访问许可）
  THEN
    查询 AccessPermit (permit_id)
    验证 Permit.IsValid()
    验证 Permit.CanAccessMapping(mapping)
    更新 Permit.UsageCount++, LastUsedAt
    允许访问
    
ELSE
  拒绝访问（缺少凭证）
```

---

## 安全优势

### 两阶段模型的安全好处

| 场景 | 传统SecretKey | 一阶段AuthCode | 两阶段AuthCode+Permit |
|------|--------------|----------------|---------------------|
| **分享安全** | ❌ 泄露永久有效 | ⚠️ 泄露10天有效 | ✅ 泄露10分钟有效 |
| **激活控制** | ❌ 无激活概念 | ❌ 无激活概念 | ✅ 需激活后才能用 |
| **使用追踪** | ❌ 无法追踪 | ⚠️ 可追踪但无Source信息 | ✅ 完整追踪Source |
| **撤销粒度** | ❌ 撤销需删映射 | ⚠️ 撤销码，但无法撤销已访问 | ✅ 撤销许可，立即生效 |
| **时间控制** | ❌ 永久 | ⚠️ 单一时效 | ✅ 双重时效（激活+访问） |

### 攻击场景分析

**场景1: 授权码被截获**
```
攻击者拦截到 "abc-def-123"

传统SecretKey:
  ❌ 永久有效，攻击者可随时使用

一阶段AuthCode (10天有效):
  ⚠️ 攻击者有10天时间利用

两阶段AuthCode+Permit:
  ✅ 攻击者只有10分钟窗口
  ✅ 如果合法用户先激活，攻击者无法再使用
  ✅ TargetClient可以看到激活记录，发现异常
```

**场景2: 访问许可被窃取**
```
攻击者窃取了 permit_xxx

传统SecretKey:
  ❌ 无SourceClient绑定，任何人能用

两阶段Permit:
  ✅ Permit绑定了SourceClient (77777777)
  ✅ 验证时检查ClientID是否匹配
  ✅ 攻击者无法冒充77777777（需要Client私钥）
```

---

## 业务场景

### 场景1: 快速分享（朋友间）

```
Alice (88888888) 想让 Bob 访问自己的家庭NAS

1. Alice: 生成授权码
   tunnox auth create --activation-ttl 10m --access-duration 1d --desc "给Bob访问NAS"
   
   得到: xyz-789-abc

2. Alice 通过微信发给 Bob: "用这个码连我的NAS: xyz-789-abc"

3. Bob: 激活
   tunnox auth activate xyz-789-abc
   
   得到: permit_bob_nas

4. Bob: 创建隧道
   tunnox mapping create --target 88888888 --permit permit_bob_nas

5. Bob: 使用1天，第二天自动过期

安全性:
- 微信被黑？没关系，10分钟后码就失效了
- Bob激活后，其他人拿到码也无法再激活
- Alice可以随时撤销Bob的访问许可
```

### 场景2: 企业临时访问（运维）

```
DevOps需要临时访问生产环境服务器进行故障排查

1. 管理员生成24小时临时授权码:
   tunnox auth create \
     --source {devops_client_id} \
     --activation-ttl 5m \
     --access-duration 24h \
     --desc "故障排查临时权限"

2. 通过安全通道（如企业IM）发送给DevOps

3. DevOps在5分钟内激活，获得24小时访问权

4. 故障解决后，管理员主动撤销许可

审计:
- 清楚记录谁在什么时候激活
- 记录访问次数和时间
- 可追溯问题责任
```

### 场景3: IoT设备授权（自动化）

```
智能家居中，手机App需要访问家庭网关

1. 网关生成长期授权码（用于设备配对）:
   tunnox auth create \
     --activation-ttl 1h \
     --access-duration 1year \
     --desc "手机App配对"

2. 用户在App中输入授权码（或扫描二维码）

3. App激活后，获得1年的访问许可

4. App后续访问直接使用许可，无需再输入码

5. 用户换手机？撤销旧许可，App重新配对

管理:
- 用户可在网关界面看到哪些设备在访问
- 可一键撤销特定设备的访问权
- 丢失设备不会泄露长期凭证（许可可撤销）
```

---

## 实施检查清单

### 数据模型 ✅
- [x] TunnelAuthCode 结构体（两阶段时效）
- [x] TunnelAccessPermit 结构体
- [x] IsValid(), CanActivate(), CanAccessMapping() 方法

### Repository 层
- [ ] AuthCodeRepository
  - [ ] CreateAuthCode
  - [ ] GetAuthCodeByCode
  - [ ] GetAuthCodesByTarget
  - [ ] ActivateAuthCode
  - [ ] RevokeAuthCode
- [ ] AccessPermitRepository
  - [ ] CreatePermit
  - [ ] GetPermit
  - [ ] ListPermitsBySource
  - [ ] ListPermitsByTarget
  - [ ] RevokePermit
  - [ ] UpdatePermitUsage

### Service 层
- [ ] AuthCodeService
  - [ ] GenerateAuthCode (包含Code生成)
  - [ ] ActivateAuthCode (创建Permit)
  - [ ] ListAuthCodes
  - [ ] RevokeAuthCode
  - [ ] CleanupExpiredCodes
- [ ] AccessPermitService
  - [ ] ValidatePermit
  - [ ] ListPermits
  - [ ] RevokePermit
  - [ ] GetPermitStats

### API 层
- [ ] POST /api/auth-codes (创建授权码)
- [ ] POST /api/auth-codes/{code}/activate (激活)
- [ ] GET /api/auth-codes (列表)
- [ ] DELETE /api/auth-codes/{code} (撤销码)
- [ ] GET /api/permits (列表)
- [ ] DELETE /api/permits/{id} (撤销许可)
- [ ] GET /api/permits/{id}/stats (统计)

### 隧道验证集成
- [ ] 扩展 TunnelOpenRequest (添加 permit_id)
- [ ] HandleTunnelOpen 集成许可验证
- [ ] 更新使用统计

### CLI 命令
- [ ] tunnox auth create
- [ ] tunnox auth activate
- [ ] tunnox auth list-codes
- [ ] tunnox auth list-permits
- [ ] tunnox auth revoke-permit
- [ ] tunnox auth info-code

### 测试
- [ ] 单元测试: 数据模型方法
- [ ] 单元测试: Repository层
- [ ] 单元测试: Service层
- [ ] 单元测试: API层
- [ ] E2E测试: 完整授权流程
- [ ] E2E测试: 撤销场景
- [ ] E2E测试: 过期场景

---

**这个两阶段授权模型是否符合您的业务需求？**

