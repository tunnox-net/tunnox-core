# Tunnox UDP 集成测试

这个目录包含了 Tunnox UDP 功能的集成测试脚本。

## 文件说明

- `config.yaml` - 测试配置文件（服务器、客户端配置）
- `private.key` - SSH 私钥（用于连接远程服务器）
- `integration_test.py` - 完整集成测试脚本（包含部署）
- `quick_test.py` - 快速测试脚本（仅测试连接）
- `README.md` - 本文档

## 环境要求

### Python 依赖

```bash
pip3 install pymysql pyyaml
```

### 系统要求

- Python 3.6+
- Go 1.19+ (用于编译)
- SSH 访问权限（用于部署到远程服务器）
- 本地 MySQL 客户端库

## 配置说明

### config.yaml 结构

```yaml
server:
  ssh:
    address: 150.109.120.165      # 远程服务器地址
    port: 22                       # SSH 端口
    user-name: root                # SSH 用户名
    ssh-key: ./private.key         # SSH 私钥路径
  tunnox-server:
    service: tunnox.service        # systemd 服务名
    config: /opt/tunnox/config.yaml # 服务器配置文件路径
    domain: gw.tunnox.net          # 服务器域名
    port: 8000                     # UDP 端口
  # UDP 加密配置（服务器端）
  udp_crypto:
    enabled: true                  # 启用加密
    plaintext_mode: false          # 禁用明文模式
    psk: ""                        # 预共享密钥（可选）
    algorithm: "chacha20-poly1305" # 加密算法
    key_rotation:                  # 密钥轮换配置
      time_threshold: 3600
      packet_threshold: 1000000
      max_retries: 3
      retry_interval: 5

listen-client:
  folder: ~/tunnox-test/listen-client  # 客户端目录
  client-id: 91400450                   # 客户端 ID
  secret-key: YvREvVjMNPmjQdc9LLTbDdT4wXRppr8h  # 密钥
  mysql-listen:
    address: 127.0.0.1             # MySQL 监听地址
    port: 9988                     # MySQL 监听端口
    user-name: root                # MySQL 用户名
    password: dtcpay               # MySQL 密码
    sql-script: select * from log.log_db_record limit 10000;  # 测试 SQL
  # UDP 加密配置（Listen Client）
  udp_crypto:
    enabled: true                  # 启用加密（与服务器保持一致）
    plaintext_mode: false          # 禁用明文模式（与服务器保持一致）
    psk: ""                        # 预共享密钥（与服务器保持一致）

target-client:
  folder: ~/tunnox-test/target-client   # 客户端目录
  client-id: 97786644                    # 客户端 ID
  secret-key: 8DlXq6GnUiDkMZTYPnxs4qfrC7RBZiEE  # 密钥
  # UDP 加密配置（Target Client）
  udp_crypto:
    enabled: true                  # 启用加密（与服务器保持一致）
    plaintext_mode: false          # 禁用明文模式（与服务器保持一致）
    psk: ""                        # 预共享密钥（与服务器保持一致）
```

### UDP 加密配置说明

#### 配置模式

1. **生产模式（推荐）**
   ```yaml
   udp_crypto:
     enabled: true
     plaintext_mode: false
     psk: ""
   ```
   - 使用 ChaCha20-Poly1305 AEAD 加密
   - 提供机密性和完整性保护
   - 适用于生产环境

2. **测试模式（仅用于调试）**
   ```yaml
   udp_crypto:
     enabled: false
     plaintext_mode: true
     psk: ""
   ```
   - ⚠️ 不加密数据，仅使用校验和
   - 仅用于测试和性能分析
   - 不应在生产环境使用

3. **PSK 模式（额外认证）**
   ```yaml
   udp_crypto:
     enabled: true
     plaintext_mode: false
     psk: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
   ```
   - 使用预共享密钥进行额外认证
   - PSK 必须是 64 位十六进制字符（32 字节）
   - 服务器和所有客户端必须使用相同的 PSK

#### 配置一致性要求

⚠️ **重要**：服务器和所有客户端的加密配置必须完全匹配！

- `server.udp_crypto` 必须与 `listen-client.udp_crypto` 匹配
- `server.udp_crypto` 必须与 `target-client.udp_crypto` 匹配
- 所有三个组件的 `enabled`、`plaintext_mode` 和 `psk` 必须相同

如果配置不匹配，连接会失败并显示认证错误。

## 使用方法

### 1. 完整集成测试（推荐）

执行完整的集成测试，包括：
- 停止现有服务
- 重新编译 server 和 client
- 部署到远程服务器和本地
- 生成配置文件
- 启动服务
- 执行 MySQL 连接测试（3次）

```bash
cd udp-test
chmod +x integration_test.py
./integration_test.py
```

或者：

```bash
python3 udp-test/integration_test.py
```

### 2. 快速测试

如果服务已经在运行，只想测试连接：

```bash
cd udp-test
chmod +x quick_test.py
./quick_test.py
```

## 测试流程

### 完整集成测试流程

1. **停止现有进程**
   - 停止远程服务器上的 tunnox.service
   - 停止本地的客户端进程
   - 如果进程无法正常停止，使用 kill -9 强制终止

2. **编译二进制文件**
   - 编译 server: `go build -o bin/server ./cmd/server`
   - 编译 client: `go build -o bin/client ./cmd/client`

3. **部署 server**
   - 通过 SCP 上传 server 到远程服务器 `/opt/tunnox/server`
   - 设置执行权限

4. **部署客户端**
   - 复制 client 到 listen-client 目录
   - 复制 client 到 target-client 目录

5. **生成配置文件**
   - 为 listen-client 生成 `client-config.yaml`
   - 为 target-client 生成 `client-config.yaml`

6. **启动服务器**
   - 启动远程服务器上的 tunnox.service
   - 检查服务状态

7. **启动客户端**
   - 启动 target-client（后台运行）
   - 启动 listen-client（后台运行）
   - 等待连接建立

8. **执行 MySQL 测试**
   - 连接到 listen-client 暴露的 MySQL 端口（9988）
   - 执行配置的 SQL 查询
   - 重复 3 次
   - 所有 3 次测试都通过才算成功

### 测试验证

测试会验证以下内容：

1. **连接测试**: `SELECT 1` 验证基本连接
2. **数据查询**: 执行配置的 SQL 查询（默认查询 10000 行）
3. **数据传输**: 验证大数据包通过 UDP 隧道的传输
4. **稳定性**: 连续 3 次测试都成功

## 测试结果

### 成功输出示例

```
✓ 连接测试成功: (1,)
✓ 查询成功! 返回 10000 行，耗时 2.35s
ℹ 估算数据大小: ~15.23 MB
✓ 第 1 轮测试通过

✓ 所有测试通过! (3/3)
```

### 失败处理

如果测试失败，脚本会：
1. 显示详细的错误信息
2. 输出服务器和客户端的日志（最后 20 行）
3. 返回非零退出码

## 故障排查

### 1. SSH 连接失败

```bash
# 检查 SSH 密钥权限
chmod 600 udp-test/private.key

# 手动测试 SSH 连接
ssh -i udp-test/private.key root@150.109.120.165
```

### 2. 编译失败

```bash
# 检查 Go 环境
go version

# 手动编译测试
go build -o bin/server ./cmd/server
go build -o bin/client ./cmd/client
```

### 3. MySQL 连接失败

```bash
# 检查客户端是否运行
ps aux | grep tunnox

# 检查端口是否监听
lsof -i :9988

# 查看客户端日志
tail -f ~/tunnox-test/listen-client/logs/client.log
tail -f ~/tunnox-test/target-client/logs/client.log
```

### 4. 服务器启动失败

```bash
# 查看服务状态
ssh -i udp-test/private.key root@150.109.120.165 'systemctl status tunnox.service'

# 查看服务日志
ssh -i udp-test/private.key root@150.109.120.165 'journalctl -u tunnox.service -n 50'
```

## 手动测试

如果需要手动测试，可以按以下步骤：

### 1. 启动 target-client

```bash
cd ~/tunnox-test/target-client
./client -config client-config.yaml -daemon
```

### 2. 启动 listen-client

```bash
cd ~/tunnox-test/listen-client
./client -config client-config.yaml -daemon
```

### 3. 测试 MySQL 连接

```bash
mysql -h 127.0.0.1 -P 9988 -u root -pdtcpay -e "SELECT 1"
```

## 日志位置

- **服务器日志**: `journalctl -u tunnox.service`
- **listen-client 日志**: `~/tunnox-test/listen-client/logs/client.log`
- **target-client 日志**: `~/tunnox-test/target-client/logs/client.log`

## 注意事项

1. **SSH 密钥安全**: 确保 `private.key` 文件权限为 600
2. **端口冲突**: 确保 9988 端口没有被其他程序占用
3. **网络连接**: 确保可以访问 gw.tunnox.net:8000
4. **MySQL 权限**: 确保配置的 MySQL 用户有查询权限
5. **磁盘空间**: 确保有足够的空间存储日志文件

## 持续集成

可以将此脚本集成到 CI/CD 流程中：

```bash
# 在 CI 环境中运行
python3 udp-test/integration_test.py
if [ $? -eq 0 ]; then
    echo "Integration test passed"
else
    echo "Integration test failed"
    exit 1
fi
```

## 扩展

### 添加更多测试

可以在 `integration_test.py` 中添加更多测试函数：

```python
def test_large_data_transfer(config):
    """测试大数据传输"""
    # 实现测试逻辑
    pass

def test_connection_stability(config):
    """测试连接稳定性"""
    # 实现测试逻辑
    pass
```

### 自定义测试参数

可以通过命令行参数自定义测试：

```python
import argparse

parser = argparse.ArgumentParser()
parser.add_argument('--rounds', type=int, default=3, help='测试轮次')
parser.add_argument('--timeout', type=int, default=30, help='超时时间')
args = parser.parse_args()
```

## 许可证

与 Tunnox 项目相同
