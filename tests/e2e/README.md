# E2E 负载均衡测试

## 概览

这个目录包含 Tunnox 的端到端 (E2E) 负载均衡测试，用于验证在多Server实例 + Nginx负载均衡器的分布式环境下系统的正确性和性能。

## 架构

```
                    ┌──────────────┐
                    │   Nginx LB   │
                    └──────┬───────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
    ┌──────▼─────┐  ┌─────▼──────┐ ┌─────▼──────┐
    │ Server-1   │  │ Server-2   │ │ Server-3   │
    └──────┬─────┘  └─────┬──────┘ └─────┬──────┘
           │              │              │
           └──────────────┼──────────────┘
                          │
                   ┌──────▼───────┐
                   │    Redis     │
                   └──────────────┘
```

## 前置条件

1. **Docker & Docker Compose**
   ```bash
   docker --version  # >= 20.10
   docker-compose --version  # >= 1.29
   ```

2. **Go环境**
   ```bash
   go version  # >= 1.24
   ```

3. **足够的系统资源**
   - CPU: 至少 4 核
   - 内存: 至少 8GB
   - 磁盘: 至少 10GB 可用空间

## 快速开始

### 1. 构建测试镜像

```bash
# 在项目根目录
cd /Users/roger.tong/GolandProjects/tunnox-core

# 构建Server镜像
docker build -f tests/e2e/Dockerfile.server -t tunnox-server:test .
```

### 2. 运行环境测试

```bash
# 运行基础环境测试
go test -v ./tests/e2e/... -run TestLoadBalancer_Environment

# 运行负载分布测试
go test -v ./tests/e2e/... -run TestLoadBalancer_BasicDistribution

# 运行并发测试
go test -v ./tests/e2e/... -run TestLoadBalancer_ConcurrentRequests

# 运行故障转移测试
go test -v ./tests/e2e/... -run TestLoadBalancer_ServiceFailover

# 运行压力测试
go test -v ./tests/e2e/... -run TestLoadBalancer_StressTest
```

### 3. 运行所有测试

```bash
# 运行所有E2E测试（可能需要30-60分钟）
go test -v ./tests/e2e/... -timeout 60m
```

## 测试用例

### 基础测试 (`load_balancer_basic_test.go`)

1. **TestLoadBalancer_Environment** - 环境基础功能
   - 验证所有服务启动成功
   - 验证健康检查正常
   - 验证Redis连接正常

2. **TestLoadBalancer_BasicDistribution** - 基本负载分布
   - 验证请求均匀分布到3个Server
   - 验证负载均衡策略生效

3. **TestLoadBalancer_ConcurrentRequests** - 并发请求
   - 100个并发请求
   - 验证成功率 > 90%

4. **TestLoadBalancer_ServiceFailover** - 服务故障转移
   - 停止1个Server
   - 验证请求自动转发到其他Server
   - 验证成功率 > 80%

5. **TestLoadBalancer_MultipleServerFailures** - 多服务器故障
   - 停止2个Server
   - 验证剩余1个Server仍可提供服务
   - 验证成功率 > 70%

6. **TestLoadBalancer_StressTest** - 压力测试
   - 50个并发worker，持续10秒
   - 验证成功率 > 95%
   - 验证QPS > 10

## 文件说明

```
tests/e2e/
├── docker-compose.load-balancer.yml  # Docker Compose配置
├── Dockerfile.server                  # Server镜像Dockerfile
├── helpers.go                         # 测试辅助工具
├── load_balancer_basic_test.go       # 基础负载均衡测试
├── nginx/
│   ├── load-balancer.conf            # Nginx负载均衡配置
│   └── html/
│       └── index.html                # 测试页面
└── README.md                          # 本文件
```

## 手动测试

### 启动环境

```bash
cd tests/e2e

# 启动所有服务
docker-compose -f docker-compose.load-balancer.yml up -d

# 查看服务状态
docker-compose -f docker-compose.load-balancer.yml ps

# 查看日志
docker-compose -f docker-compose.load-balancer.yml logs -f
```

### 测试负载均衡

```bash
# 测试Nginx健康检查
curl http://localhost:8080/health

# 多次请求，观察负载分布
for i in {1..10}; do
    curl -s http://localhost:8080/health
    echo ""
done
```

### 测试故障转移

```bash
# 停止Server-1
docker-compose -f docker-compose.load-balancer.yml stop tunnox-server-1

# 继续请求，应该转发到Server-2和Server-3
curl http://localhost:8080/health

# 重启Server-1
docker-compose -f docker-compose.load-balancer.yml start tunnox-server-1
```

### 清理环境

```bash
# 停止并删除所有容器和卷
docker-compose -f docker-compose.load-balancer.yml down -v

# 删除镜像（可选）
docker rmi tunnox-server:test
```

## 故障排查

### 服务无法启动

1. 检查端口是否被占用：
   ```bash
   lsof -i :7000
   lsof -i :7001
   lsof -i :8080
   lsof -i :6379
   ```

2. 查看服务日志：
   ```bash
   docker-compose -f docker-compose.load-balancer.yml logs tunnox-server-1
   docker-compose -f docker-compose.load-balancer.yml logs redis
   docker-compose -f docker-compose.load-balancer.yml logs nginx
   ```

### 健康检查失败

1. 检查Server日志：
   ```bash
   docker logs <container_id>
   ```

2. 进入容器检查：
   ```bash
   docker exec -it <container_id> sh
   wget -O- http://localhost:8080/health
   ```

### Redis连接失败

1. 检查Redis服务：
   ```bash
   docker exec -it <redis_container_id> redis-cli ping
   ```

2. 检查网络连接：
   ```bash
   docker network ls
   docker network inspect tunnox-net
   ```

### 测试超时

1. 增加测试超时时间：
   ```bash
   go test -v ./tests/e2e/... -timeout 120m
   ```

2. 减少测试规模（修改测试代码中的并发数和持续时间）

## 性能基准

### 目标性能指标

| 指标 | 单节点 | 3节点集群 | 
|------|--------|----------|
| **最大并发连接** | 10,000 | 30,000 |
| **平均延迟** | <10ms | <20ms |
| **成功率** | >99% | >95% |
| **QPS** | 1000/s | 2500/s |

## 贡献指南

添加新的E2E测试时，请遵循以下规范：

1. **测试命名**: `TestLoadBalancer_<Feature>` 格式
2. **Short模式跳过**: 添加 `if testing.Short() { t.Skip() }`
3. **日志输出**: 使用 emoji 和清晰的日志信息
4. **资源清理**: 使用 `defer compose.Cleanup()`
5. **断言**: 使用 `testify/assert` 和 `testify/require`
6. **文档**: 在README中添加测试说明

## 相关文档

- [E2E 负载均衡器测试计划](../../docs/E2E_LOAD_BALANCER_TEST_PLAN.md)
- [测试构建计划](../../docs/TEST_CONSTRUCTION_PLAN.md)
- [架构设计](../../docs/ARCHITECTURE_DESIGN_V2.2.md)

## 许可证

MIT

