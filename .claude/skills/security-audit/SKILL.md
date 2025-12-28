---
name: security-audit
description: 安全审计技能。审查隧道系统的安全性，包括认证、加密、权限控制、漏洞检测。关键词：安全、审计、加密、认证、漏洞、权限。
allowed-tools: Read, Grep, Glob, Bash
---

# 安全审计技能

## 安全审计范围

### 1. 认证与授权

- JWT Token 安全性
- 客户端认证机制
- 权限控制粒度
- 会话管理

### 2. 数据传输安全

- 传输层加密
- 端到端加密
- 密钥管理
- 防重放攻击

### 3. 基础设施安全

- 端口暴露
- 配置安全
- 日志脱敏
- 依赖安全

## 安全检查清单

### 认证安全

```markdown
## JWT Token 安全

- [ ] **密钥强度**: jwt_secret_key 至少 32 字节
- [ ] **算法安全**: 使用 HS256 或更强算法
- [ ] **过期时间**: Token 有合理的过期时间
- [ ] **刷新机制**: 支持 Token 刷新
- [ ] **吊销机制**: 支持 Token 吊销

检查命令:
grep -r "jwt_secret\|secret_key" config/
grep -r "jwt.SigningMethod\|HS256\|RS256" internal/
```

```markdown
## 客户端认证

- [ ] **设备 ID**: 匿名模式设备 ID 不可预测
- [ ] **Token 传输**: 认证信息不通过 URL 传输
- [ ] **暴力破解**: 有登录失败限制
- [ ] **会话固定**: 登录后更换会话 ID

检查命令:
grep -r "BruteForce\|rate.Limit\|LoginAttempt" internal/
```

### 加密安全

```markdown
## 传输加密

- [ ] **TLS 版本**: 最低 TLS 1.2
- [ ] **密码套件**: 使用强密码套件
- [ ] **证书验证**: 验证服务器证书
- [ ] **HSTS**: 启用 HSTS (如适用)

检查命令:
grep -r "tls.Config\|MinVersion\|CipherSuites" internal/
```

```markdown
## 数据加密

- [ ] **算法**: 使用 AES-256-GCM
- [ ] **密钥长度**: 密钥至少 256 位
- [ ] **IV/Nonce**: 每次加密使用随机 IV
- [ ] **密钥派生**: 使用 PBKDF2/Argon2 派生密钥

检查位置:
internal/stream/encryption/
internal/security/

检查命令:
grep -r "aes\|gcm\|cipher" internal/stream/encryption/
```

### 输入验证

```markdown
## 输入安全

- [ ] **SQL 注入**: 参数化查询
- [ ] **命令注入**: 不拼接用户输入到命令
- [ ] **路径遍历**: 验证文件路径
- [ ] **XSS**: HTML 转义 (如适用)

检查命令:
grep -r "fmt.Sprintf.*%s.*sql\|exec.Command" internal/
grep -rn "os.Open\|ioutil.ReadFile" internal/ | grep -v "_test.go"
```

### 权限控制

```markdown
## 权限检查

- [ ] **最小权限**: 服务以最小权限运行
- [ ] **权限验证**: API 调用前验证权限
- [ ] **配额限制**: 有用量配额限制
- [ ] **资源隔离**: 用户间资源隔离

检查命令:
grep -r "CheckPermission\|CheckQuota\|Authorize" internal/
```

### 日志安全

```markdown
## 日志安全

- [ ] **敏感信息**: 不记录密码、Token
- [ ] **PII 脱敏**: 个人信息脱敏
- [ ] **日志注入**: 防止日志注入
- [ ] **存储安全**: 日志文件权限正确

检查命令:
grep -rn "password\|token\|secret" internal/ | grep -i "log\|print\|fmt"
```

## 常见漏洞检测

### 硬编码凭证

```bash
# 检查硬编码密码
grep -rn "password\s*=\s*[\"']" internal/
grep -rn "secret\s*=\s*[\"']" internal/
grep -rn "apikey\s*=\s*[\"']" internal/

# 检查硬编码 Token
grep -rn "eyJ[a-zA-Z0-9]" internal/  # JWT 格式
```

### 不安全的随机数

```bash
# 检查不安全的随机数使用
grep -rn "math/rand" internal/

# 应该使用 crypto/rand
grep -rn "crypto/rand" internal/
```

### 竞态条件

```bash
# 运行竞态检测
go test -race ./...
```

### 依赖漏洞

```bash
# 检查依赖漏洞
go list -m all | xargs -n1 go list -json -m | jq -r '.Path'
# 使用 govulncheck
govulncheck ./...
```

## 安全问题模板

### 高危问题

```markdown
## [CRITICAL] 硬编码密钥

**位置**: internal/security/crypto.go:25
**问题**: AES 密钥硬编码在代码中
**影响**: 攻击者可解密所有加密数据
**CVSS**: 9.8

**当前代码**:
```go
var key = []byte("1234567890123456")  // 硬编码密钥
```

**修复建议**:
```go
// 从配置或环境变量读取
key := []byte(os.Getenv("ENCRYPTION_KEY"))
if len(key) != 32 {
    return errors.New("invalid encryption key")
}
```
```

### 中危问题

```markdown
## [HIGH] JWT 无过期时间

**位置**: internal/cloud/managers/jwt_manager.go:45
**问题**: JWT Token 没有设置过期时间
**影响**: Token 泄露后永久有效
**CVSS**: 7.5

**当前代码**:
```go
token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
// 缺少 exp claim
```

**修复建议**:
```go
claims["exp"] = time.Now().Add(24 * time.Hour).Unix()
token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
```
```

### 低危问题

```markdown
## [MEDIUM] 日志记录敏感信息

**位置**: internal/client/connection.go:89
**问题**: 日志中记录了完整的认证 Token
**影响**: Token 可能通过日志泄露
**CVSS**: 4.3

**当前代码**:
```go
utils.Infof("Connecting with token: %s", token)
```

**修复建议**:
```go
utils.Infof("Connecting with token: %s...%s", token[:8], token[len(token)-4:])
```
```

## 安全配置建议

### 生产环境配置

```yaml
# config.yaml - 安全配置示例

# TLS 配置
tls:
  enabled: true
  cert_file: "/etc/tunnox/server.crt"
  key_file: "/etc/tunnox/server.key"
  min_version: "1.2"
  cipher_suites:
    - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
    - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256

# JWT 配置
cloud:
  built_in:
    jwt_secret_key: "${JWT_SECRET}"  # 从环境变量读取
    jwt_expiration: 3600  # 1小时过期

# 安全限制
security:
  rate_limit:
    enabled: true
    requests_per_second: 100
  brute_force:
    enabled: true
    max_attempts: 5
    lockout_duration: 300
```

### 安全加固脚本

```bash
#!/bin/bash
# security-harden.sh

# 1. 检查文件权限
chmod 600 /etc/tunnox/*.key
chmod 644 /etc/tunnox/*.crt
chmod 600 /etc/tunnox/config.yaml

# 2. 检查环境变量
if [ -z "$JWT_SECRET" ]; then
    echo "ERROR: JWT_SECRET not set"
    exit 1
fi

if [ ${#JWT_SECRET} -lt 32 ]; then
    echo "ERROR: JWT_SECRET too short (min 32 chars)"
    exit 1
fi

# 3. 检查端口暴露
netstat -tlnp | grep tunnox

# 4. 检查日志权限
chmod 600 /var/log/tunnox/*.log
```

## 安全审计报告模板

```markdown
# 安全审计报告

**项目**: Tunnox Core
**版本**: v1.1.11
**审计日期**: 2025-01-28
**审计人员**: Security Team

## 审计范围

- 认证与授权
- 数据传输加密
- 输入验证
- 权限控制
- 日志安全

## 审计结果汇总

| 严重程度 | 数量 | 状态 |
|----------|------|------|
| Critical | 0 | - |
| High | 1 | 待修复 |
| Medium | 3 | 待修复 |
| Low | 5 | 建议改进 |

## 详细发现

### [HIGH] H001: JWT 密钥强度不足
- 位置: config.example.yaml
- 描述: 示例配置中的 JWT 密钥过短
- 建议: 文档中强调使用强密钥

### [MEDIUM] M001: 缺少速率限制
- 位置: internal/api/
- 描述: API 端点无速率限制
- 建议: 添加 rate limiter 中间件

## 建议优先级

1. **立即修复**: H001
2. **短期修复**: M001, M002, M003
3. **长期改进**: L001-L005

## 结论

整体安全性良好，需要修复 1 个高危问题和 3 个中危问题。
建议在下一版本发布前完成修复。
```
