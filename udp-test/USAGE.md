# 快速使用指南

## 第一次使用

### 1. 安装依赖

```bash
cd udp-test
make install
```

或者手动安装：

```bash
pip3 install pymysql pyyaml
```

### 2. 配置 SSH 密钥

确保 `private.key` 文件存在且权限正确：

```bash
chmod 600 udp-test/private.key
```

### 3. 运行完整测试

```bash
cd udp-test
make test
```

或者：

```bash
./integration_test.py
```

## 日常使用

### 快速测试（服务已运行）

```bash
cd udp-test
make quick-test
```

### 查看日志

```bash
make logs
```

### 查看进程状态

```bash
make status
```

### 清理环境

```bash
make clean
```

## 测试流程说明

### 完整测试流程

```
1. 停止现有进程
   ├─ 停止远程服务器 (systemctl stop tunnox.service)
   └─ 停止本地客户端 (pkill)

2. 编译二进制文件
   ├─ go build server
   └─ go build client

3. 部署
   ├─ SCP server 到远程服务器
   ├─ 复制 client 到 listen-client 目录
   └─ 复制 client 到 target-client 目录

4. 生成配置
   ├─ listen-client/client-config.yaml
   └─ target-client/client-config.yaml

5. 启动服务
   ├─ 启动远程服务器
   ├─ 启动 target-client
   └─ 启动 listen-client

6. 执行测试
   ├─ 第 1 轮 MySQL 测试
   ├─ 第 2 轮 MySQL 测试
   └─ 第 3 轮 MySQL 测试

7. 生成报告
```

### 快速测试流程

```
1. 连接 MySQL (127.0.0.1:9988)
2. 执行测试查询 (3 轮)
3. 生成结果
```

## 测试架构

```
┌─────────────────┐
│  本地机器       │
│                 │
│  ┌───────────┐  │
│  │ listen-   │  │
│  │ client    │  │
│  │ :9988     │  │
│  └─────┬─────┘  │
│        │ UDP    │
└────────┼────────┘
         │
         │ Internet
         │
┌────────┼────────┐
│        │        │
│  ┌─────▼─────┐  │
│  │  Server   │  │  腾讯云服务器
│  │  :8000    │  │  (gw.tunnox.net)
│  └─────┬─────┘  │
│        │ UDP    │
└────────┼────────┘
         │
         │
┌────────┼────────┐
│        │        │
│  ┌─────▼─────┐  │
│  │ target-   │  │  本地机器
│  │ client    │  │
│  └───────────┘  │
│        │        │
│  ┌─────▼─────┐  │
│  │  MySQL    │  │
│  │  :3306    │  │
│  └───────────┘  │
└─────────────────┘
```

## UDP 加密配置

### 加密模式说明

Tunnox UDP 支持三种加密模式：

1. **加密模式（生产环境推荐）**
   - `enabled: true`
   - `plaintext_mode: false`
   - 使用 ChaCha20-Poly1305 AEAD 加密
   - 提供机密性和完整性保护

2. **明文模式（仅用于测试）**
   - `enabled: false`
   - `plaintext_mode: true`
   - 不加密数据，仅使用校验和
   - ⚠️ 不应在生产环境使用

3. **PSK 模式（额外认证）**
   - `enabled: true`
   - `psk: "64位十六进制字符"`
   - 使用预共享密钥进行额外认证
   - 适用于高安全要求场景

### 测试不同加密模式

#### 测试加密模式（默认）

配置文件已默认启用加密，直接运行测试：

```bash
cd udp-test
./integration_test.py
```

#### 测试明文模式

编辑 `config.yaml`，将所有 `udp_crypto` 部分修改为：

```yaml
udp_crypto:
  enabled: false
  plaintext_mode: true
  psk: ""
```

需要修改三个位置：
- `server.udp_crypto`
- `listen-client.udp_crypto`
- `target-client.udp_crypto`

然后运行测试：

```bash
cd udp-test
./integration_test.py
```

#### 测试 PSK 模式

1. 生成 PSK（64 位十六进制字符）：

```bash
openssl rand -hex 32
```

2. 编辑 `config.yaml`，将所有 `udp_crypto.psk` 设置为相同的值：

```yaml
udp_crypto:
  enabled: true
  plaintext_mode: false
  psk: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
```

3. 运行测试：

```bash
cd udp-test
./integration_test.py
```

### 配置一致性检查

⚠️ **重要**：服务器和所有客户端的加密配置必须完全匹配！

检查清单：
- [ ] `server.udp_crypto.enabled` = `listen-client.udp_crypto.enabled` = `target-client.udp_crypto.enabled`
- [ ] `server.udp_crypto.plaintext_mode` = `listen-client.udp_crypto.plaintext_mode` = `target-client.udp_crypto.plaintext_mode`
- [ ] `server.udp_crypto.psk` = `listen-client.udp_crypto.psk` = `target-client.udp_crypto.psk`

如果配置不匹配，连接会失败并显示认证错误。

## 测试数据流

```
MySQL Client
    │
    │ SQL Query
    ▼
listen-client:9988
    │
    │ UDP Tunnel
    ▼
Server (gw.tunnox.net:8000)
    │
    │ UDP Tunnel
    ▼
target-client
    │
    │ TCP
    ▼
MySQL Server:3306
    │
    │ Result
    ▼
(返回路径相同)
```

## 常见问题

### Q: 测试失败怎么办？

A: 按以下步骤排查：

1. 查看日志
```bash
make logs
```

2. 检查进程状态
```bash
make status
```

3. 手动测试 SSH 连接
```bash
ssh -i private.key root@150.109.120.165
```

4. 检查端口
```bash
lsof -i :9988
```

### Q: 如何只重新生成配置？

A: 运行配置生成脚本：

```bash
./generate_configs.py
```

### Q: 如何手动启动客户端？

A: 

```bash
# 启动 target-client
cd ~/tunnox-test/target-client
./client -config client-config.yaml -daemon

# 启动 listen-client
cd ~/tunnox-test/listen-client
./client -config client-config.yaml -daemon
```

### Q: 如何查看实时日志？

A:

```bash
# listen-client 日志
tail -f ~/tunnox-test/listen-client/logs/client.log

# target-client 日志
tail -f ~/tunnox-test/target-client/logs/client.log

# 服务器日志
ssh -i private.key root@150.109.120.165 'journalctl -u tunnox.service -f'
```

### Q: 测试超时怎么办？

A: 可能的原因：
1. 网络连接问题 - 检查是否能访问 gw.tunnox.net:8000
2. 服务未启动 - 检查服务器和客户端状态
3. 防火墙阻止 - 检查 UDP 8000 端口是否开放
4. MySQL 查询太慢 - 减少查询的数据量

## 自定义测试

### 修改测试 SQL

编辑 `config.yaml`：

```yaml
listen-client:
  mysql-listen:
    sql-script: select * from your_table limit 1000;
```

### 修改测试轮次

编辑 `integration_test.py` 或 `quick_test.py`：

```python
total_rounds = 5  # 改为 5 轮测试
```

### 添加更多测试

在 `integration_test.py` 中添加新的测试函数：

```python
def test_performance(config):
    """性能测试"""
    # 实现测试逻辑
    pass

# 在 main() 中调用
test_performance(config)
```

## 持续集成

### GitHub Actions 示例

```yaml
name: UDP Integration Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      
      - name: Setup Python
        uses: actions/setup-python@v2
        with:
          python-version: '3.9'
      
      - name: Install dependencies
        run: |
          cd udp-test
          make install
      
      - name: Run integration test
        run: |
          cd udp-test
          ./integration_test.py
        env:
          SSH_KEY: ${{ secrets.SSH_PRIVATE_KEY }}
```

## 性能基准

预期性能指标：

- **连接建立**: < 2s
- **小查询 (1 行)**: < 0.5s
- **中等查询 (1000 行)**: < 2s
- **大查询 (10000 行)**: < 5s
- **超大查询 (30000 行)**: < 15s

如果实际性能低于这些指标，可能需要优化。

## 安全注意事项

1. **不要提交私钥**: `private.key` 已在 `.gitignore` 中
2. **保护配置文件**: 如果包含敏感信息，使用 `config.local.yaml`
3. **限制 SSH 访问**: 使用密钥而非密码
4. **定期更新密钥**: 定期轮换 SSH 密钥和客户端密钥

## 支持

如有问题，请查看：
- 主项目 README
- 项目文档目录 (`docs/`)
- 提交 Issue
