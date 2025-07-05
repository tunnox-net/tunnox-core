# 配置文件说明

## 文件结构

```
cmd/server/config/
├── config.yaml          # 服务器配置文件（实际使用）
├── config.example.yaml  # 配置文件示例
└── README.md           # 本说明文档
```

## 使用方法

### 1. 复制配置文件
```bash
# 复制示例配置文件
cp config.example.yaml config.yaml

# 根据需要修改配置
vim config.yaml
```

### 2. 启动服务器
```bash
# 使用默认配置文件
go run cmd/server/main.go

# 指定配置文件路径
go run cmd/server/main.go -config /path/to/config.yaml
```

## 配置项说明

### Server 配置
- `host`: 服务器监听地址
- `port`: 服务器监听端口
- `read_timeout`: 读取超时时间
- `write_timeout`: 写入超时时间
- `idle_timeout`: 空闲连接超时时间

### 日志配置
- `level`: 日志级别（debug, info, warn, error, fatal, panic）
- `format`: 日志格式（json, text）
- `output`: 输出位置（stdout, stderr, file）
- `file`: 日志文件路径

### 云控配置
- `type`: 云控类型（built_in, external）
- `jwt_secret_key`: JWT 签名密钥
- `jwt_expiration`: JWT 过期时间
- `refresh_expiration`: 刷新令牌过期时间

### 性能配置
- `buffer_pool`: 内存池配置
- `connection_pool`: 连接池配置

### 监控配置
- `metrics`: 指标收集配置
- `health_check`: 健康检查配置

### 安全配置
- `tls`: TLS 配置
- `auth`: 认证配置

## 环境变量支持

支持通过环境变量覆盖配置：

```bash
# 设置服务器端口
export TUNNOX_SERVER_PORT=9090

# 设置日志级别
export TUNNOX_LOG_LEVEL=debug

# 设置 JWT 密钥
export TUNNOX_CLOUD_BUILT_IN_JWT_SECRET_KEY=your-secret-key
```

## 配置文件优先级

1. 命令行参数（最高优先级）
2. 环境变量
3. 配置文件
4. 默认值（最低优先级）

## 注意事项

1. **生产环境**：请修改 `jwt_secret_key` 为强密钥
2. **安全**：生产环境建议启用 TLS
3. **日志**：生产环境建议使用文件输出并配置日志轮转
4. **监控**：建议启用指标收集和健康检查

## 示例配置

### 开发环境
```yaml
server:
  port: 8080
log:
  level: debug
  output: stdout
development:
  debug: true
```

### 生产环境
```yaml
server:
  port: 443
  tls:
    enabled: true
log:
  level: info
  output: file
  file: /var/log/tunnox/server.log
security:
  auth:
    admin:
      password: "strong-password-here"
``` 