# Tunnox Core Docker 部署指南

## GitHub Actions 自动构建

### 配置 GitHub Secrets

在 GitHub 仓库的 Settings > Secrets and variables > Actions 中添加以下 secrets：

| Secret 名称 | 说明 | 示例值 |
|------------|------|--------|
| `DOCKER_REGISTRY_USERNAME` | 镜像仓库用户名 | `your-username` |
| `DOCKER_REGISTRY_PASSWORD` | 镜像仓库密码 | `your-password` |

### 触发构建

工作流会在以下情况自动触发：

1. **推送到 main/develop 分支**：生成 `latest` 或分支名标签
2. **创建 tag**：推送 `v1.0.0` 格式的 tag 会生成对应版本镜像
3. **Pull Request**：仅构建不推送

### 镜像标签规则

**Server 镜像**：
- `cr.cnkl.org/tunnox/tunnox-server:latest` - main 分支最新版本
- `cr.cnkl.org/tunnox/tunnox-server:v1.0.0` - 版本标签
- `cr.cnkl.org/tunnox/tunnox-server:1.0` - 主次版本
- `cr.cnkl.org/tunnox/tunnox-server:1` - 主版本
- `cr.cnkl.org/tunnox/tunnox-server:main-abc1234` - 分支+commit SHA

**Client 镜像**：
- `cr.cnkl.org/tunnox/tunnox-client:latest` - main 分支最新版本
- `cr.cnkl.org/tunnox/tunnox-client:v1.0.0` - 版本标签
- `cr.cnkl.org/tunnox/tunnox-client:1.0` - 主次版本
- `cr.cnkl.org/tunnox/tunnox-client:1` - 主版本
- `cr.cnkl.org/tunnox/tunnox-client:main-abc1234` - 分支+commit SHA

## 本地构建

```bash
# 构建 Server 镜像
docker build -t cr.cnkl.org/tunnox/tunnox-server:v1.0.0 -f Dockerfile .

# 构建 Client 镜像
docker build -t cr.cnkl.org/tunnox/tunnox-client:v1.0.0 -f Dockerfile.client .

# 登录镜像仓库
docker login cr.cnkl.org

# 推送镜像
docker push cr.cnkl.org/tunnox/tunnox-server:v1.0.0
docker push cr.cnkl.org/tunnox/tunnox-client:v1.0.0
```

## 运行容器

### Server 容器

#### 基本运行（内存存储模式）

```bash
docker run -d \
  --name tunnox-server \
  -p 7000:7000 \
  -p 8000:8000 \
  -p 9000:9000 \
  cr.cnkl.org/tunnox/tunnox-server:latest
```

#### 使用自定义配置

```bash
docker run -d \
  --name tunnox-server \
  -p 7000:7000 \
  -p 8000:8000 \
  -p 9000:9000 \
  -v /path/to/config.yaml:/app/config/config.yaml \
  cr.cnkl.org/tunnox/tunnox-server:latest \
  -config /app/config/config.yaml
```

#### 集群模式（使用 Redis）

```bash
docker run -d \
  --name tunnox-server \
  -p 7000:7000 \
  -p 8000:8000 \
  -p 9000:9000 \
  -e STORAGE_TYPE=redis \
  -e REDIS_ADDRESS=redis:6379 \
  -e REDIS_PASSWORD=your-password \
  -e MESSAGE_BROKER_TYPE=redis \
  cr.cnkl.org/tunnox/tunnox-server:latest
```

### Client 容器

#### TCP 协议连接

```bash
docker run -d \
  --name tunnox-client \
  -e TUNNOX_SERVER=server-ip:8000 \
  -e TUNNOX_PROTOCOL=tcp \
  cr.cnkl.org/tunnox/tunnox-client:latest
```

#### WebSocket 协议连接

```bash
docker run -d \
  --name tunnox-client \
  -e TUNNOX_SERVER=server-ip:8443 \
  -e TUNNOX_PROTOCOL=websocket \
  cr.cnkl.org/tunnox/tunnox-client:latest
```

#### KCP 协议连接（低延迟）

```bash
docker run -d \
  --name tunnox-client \
  -e TUNNOX_SERVER=server-ip:8000 \
  -e TUNNOX_PROTOCOL=kcp \
  cr.cnkl.org/tunnox/tunnox-client:latest
```

#### QUIC 协议连接（高性能）

```bash
docker run -d \
  --name tunnox-client \
  -e TUNNOX_SERVER=server-ip:443 \
  -e TUNNOX_PROTOCOL=quic \
  cr.cnkl.org/tunnox/tunnox-client:latest
```

## 环境变量

### Server 环境变量

| 变量名 | 说明 | 默认值 | 必填 |
|--------|------|--------|------|
| `STORAGE_TYPE` | 存储类型 (memory/redis) | `memory` | 否 |
| `REDIS_ADDRESS` | Redis 地址 | - | 使用 Redis 时必填 |
| `REDIS_PASSWORD` | Redis 密码 | - | 否 |
| `MESSAGE_BROKER_TYPE` | 消息代理类型 (memory/redis) | `memory` | 否 |
| `NODE_ID` | 节点 ID（集群模式） | 自动分配 | 否 |
| `JWT_SECRET_KEY` | JWT 密钥 | - | 生产环境必填 |

### Client 环境变量

| 变量名 | 说明 | 默认值 | 必填 |
|--------|------|--------|------|
| `TUNNOX_SERVER` | 服务器地址 | `core:7000` | 否 |
| `TUNNOX_PROTOCOL` | 传输协议 (tcp/websocket/kcp/quic) | `tcp` | 否 |

## Docker Compose 示例

### 单机部署（内存存储）

```yaml
version: '3.8'

services:
  tunnox-server:
    image: cr.cnkl.org/tunnox/tunnox-server:latest
    ports:
      - "7000:7000"   # TCP 客户端连接
      - "8000:8000"   # WebSocket
      - "9000:9000"   # Management API
    environment:
      STORAGE_TYPE: memory
      MESSAGE_BROKER_TYPE: memory
      JWT_SECRET_KEY: "change-this-in-production"
    restart: unless-stopped

  tunnox-client-1:
    image: cr.cnkl.org/tunnox/tunnox-client:latest
    depends_on:
      - tunnox-server
    environment:
      TUNNOX_SERVER: "tunnox-server:8000"
      TUNNOX_PROTOCOL: "tcp"
    restart: unless-stopped

  tunnox-client-2:
    image: cr.cnkl.org/tunnox/tunnox-client:latest
    depends_on:
      - tunnox-server
    environment:
      TUNNOX_SERVER: "tunnox-server:8000"
      TUNNOX_PROTOCOL: "tcp"
    restart: unless-stopped
```

### 集群部署（Redis 存储）

```yaml
version: '3.8'

services:
  redis:
    image: redis:7-alpine
    command: redis-server --requirepass tunnox123
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    restart: unless-stopped

  tunnox-server-1:
    image: cr.cnkl.org/tunnox/tunnox-server:latest
    depends_on:
      - redis
    ports:
      - "7001:7000"
      - "8001:8000"
      - "9001:9000"
    environment:
      STORAGE_TYPE: redis
      REDIS_ADDRESS: "redis:6379"
      REDIS_PASSWORD: "tunnox123"
      MESSAGE_BROKER_TYPE: redis
      JWT_SECRET_KEY: "change-this-in-production"
    restart: unless-stopped

  tunnox-server-2:
    image: cr.cnkl.org/tunnox/tunnox-server:latest
    depends_on:
      - redis
    ports:
      - "7002:7000"
      - "8002:8000"
      - "9002:9000"
    environment:
      STORAGE_TYPE: redis
      REDIS_ADDRESS: "redis:6379"
      REDIS_PASSWORD: "tunnox123"
      MESSAGE_BROKER_TYPE: redis
      JWT_SECRET_KEY: "change-this-in-production"
    restart: unless-stopped

  tunnox-client:
    image: cr.cnkl.org/tunnox/tunnox-client:latest
    depends_on:
      - tunnox-server-1
      - tunnox-server-2
    environment:
      TUNNOX_SERVER: "tunnox-server-1:8000"
      TUNNOX_PROTOCOL: "tcp"
    restart: unless-stopped

volumes:
  redis_data:
```

### 完整测试环境（包含 MySQL、Nginx）

```yaml
version: '3.8'

services:
  # Redis 存储
  redis:
    image: redis:7-alpine
    command: redis-server --requirepass tunnox123
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

  # Tunnox Server
  tunnox-server:
    image: cr.cnkl.org/tunnox/tunnox-server:latest
    depends_on:
      - redis
    ports:
      - "7000:7000"
      - "8000:8000"
      - "9000:9000"
    environment:
      STORAGE_TYPE: redis
      REDIS_ADDRESS: "redis:6379"
      REDIS_PASSWORD: "tunnox123"
      MESSAGE_BROKER_TYPE: redis
      JWT_SECRET_KEY: "test-secret-key"

  # MySQL 测试数据库
  mysql:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: root123
      MYSQL_DATABASE: testdb
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql

  # Nginx 测试服务
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro

  # 目标端客户端（连接到 MySQL）
  tunnox-client-target:
    image: cr.cnkl.org/tunnox/tunnox-client:latest
    depends_on:
      - tunnox-server
      - mysql
    environment:
      TUNNOX_SERVER: "tunnox-server:8000"
      TUNNOX_PROTOCOL: "tcp"

  # 源端客户端
  tunnox-client-source:
    image: cr.cnkl.org/tunnox/tunnox-client:latest
    depends_on:
      - tunnox-server
    environment:
      TUNNOX_SERVER: "tunnox-server:8000"
      TUNNOX_PROTOCOL: "tcp"
    ports:
      - "13306:13306"  # 映射 MySQL 端口

volumes:
  redis_data:
  mysql_data:
```

## 版本发布流程

1. 更新代码并提交到 develop 分支
2. 测试通过后合并到 main 分支
3. 创建版本标签：
   ```bash
   git tag -a v1.0.0 -m "Release version 1.0.0"
   git push origin v1.0.0
   ```
4. GitHub Actions 自动构建并推送 Server 和 Client 镜像
5. 使用新版本镜像部署

## 健康检查

### Server 健康检查

```bash
# 通过 Management API 检查
curl http://localhost:9000/tunnox/v1/health

# 或使用 Docker 健康检查
docker inspect --format='{{.State.Health.Status}}' tunnox-server
```

### Client 健康检查

```bash
# 查看客户端日志
docker logs tunnox-client

# 进入容器检查
docker exec -it tunnox-client sh
```

## 端口说明

### Server 端口

| 端口 | 协议 | 说明 |
|------|------|------|
| 7000 | TCP | TCP 客户端连接 |
| 8000 | TCP/UDP | WebSocket / KCP |
| 8443 | TCP | WebSocket (TLS) |
| 443 | UDP | QUIC |
| 9000 | HTTP | Management API |

### Client 端口

Client 容器不需要暴露端口，除非需要从宿主机访问映射的服务。

## 故障排查

### 查看日志

```bash
# Server 日志
docker logs tunnox-server

# Client 日志
docker logs tunnox-client

# 实时查看日志
docker logs -f tunnox-server
```

### 进入容器

```bash
# 进入 Server 容器
docker exec -it tunnox-server sh

# 进入 Client 容器
docker exec -it tunnox-client sh
```

### 测试连接

```bash
# 测试 TCP 端口
nc -zv server-ip 7000

# 测试 WebSocket
curl -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" \
  http://server-ip:8000/

# 测试 Management API
curl http://server-ip:9000/tunnox/v1/health
```

### 常见问题

**问题：客户端无法连接到服务器**

解决方案：
1. 检查网络连接：`ping server-ip`
2. 检查端口是否开放：`nc -zv server-ip 8000`
3. 检查防火墙规则
4. 查看服务器日志：`docker logs tunnox-server`

**问题：Redis 连接失败**

解决方案：
1. 检查 Redis 是否运行：`docker ps | grep redis`
2. 测试 Redis 连接：`redis-cli -h redis -p 6379 -a password ping`
3. 检查环境变量配置是否正确

**问题：镜像拉取失败**

解决方案：
1. 登录镜像仓库：`docker login cr.cnkl.org`
2. 检查网络连接
3. 确认镜像名称和标签是否正确

## 性能优化

### 资源限制

```yaml
services:
  tunnox-server:
    image: cr.cnkl.org/tunnox/tunnox-server:latest
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G
```

### 网络优化

```yaml
services:
  tunnox-server:
    image: cr.cnkl.org/tunnox/tunnox-server:latest
    network_mode: host  # 使用宿主机网络，减少 NAT 开销
```

### 日志管理

```yaml
services:
  tunnox-server:
    image: cr.cnkl.org/tunnox/tunnox-server:latest
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

## 安全建议

1. **修改默认密钥**：生产环境必须修改 `JWT_SECRET_KEY`
2. **使用 TLS**：启用 HTTPS 和 WSS
3. **限制端口访问**：使用防火墙规则限制访问
4. **定期更新**：及时更新到最新版本
5. **监控日志**：定期检查异常日志

## 监控和告警

### Prometheus 监控

```yaml
services:
  prometheus:
    image: prom/prometheus
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"

  grafana:
    image: grafana/grafana
    ports:
      - "3000:3000"
    environment:
      GF_SECURITY_ADMIN_PASSWORD: admin
```

### 日志收集

```yaml
services:
  loki:
    image: grafana/loki
    ports:
      - "3100:3100"

  promtail:
    image: grafana/promtail
    volumes:
      - /var/log:/var/log
      - ./promtail-config.yml:/etc/promtail/config.yml
```

## 下一步

- 查看 [README.md](README.md) 了解更多功能
- 查看 [docs/ARCHITECTURE_DESIGN_V2.2.md](docs/ARCHITECTURE_DESIGN_V2.2.md) 了解架构设计
- 查看 [docs/MANAGEMENT_API.md](docs/MANAGEMENT_API.md) 了解 API 使用
