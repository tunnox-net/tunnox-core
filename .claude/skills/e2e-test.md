# /e2e-test - 端到端测试

执行 Tunnox Core 的端到端测试，验证完整的隧道功能。

## 测试环境

使用 `start_test.sh` 脚本启动完整测试环境：

```bash
./start_test.sh
```

脚本执行：
1. 清理现有进程和日志
2. 构建服务端和客户端
3. 启动服务端
4. 启动两个客户端实例（target 和 listen）

## 日志位置

- 服务端: `~/GolandProjects/tunnox-core/cmd/server/logs/server.log`
- 目标端客户端: `/tmp/tunnox-target-client.log`
- 源端客户端: `/tmp/tunnox-listen-client.log`

## 测试场景

### 1. TCP 隧道测试

```bash
# 目标端生成连接码
tunnox> generate-code
# 选择 TCP，目标地址 localhost:3306

# 源端使用连接码
tunnox> use-code <code>
# 指定本地端口 127.0.0.1:13306

# 验证连接
mysql -h 127.0.0.1 -P 13306 -u root -p
```

### 2. 多协议测试

```bash
# TCP
./bin/client -s 127.0.0.1:8000 -p tcp -anonymous

# WebSocket
./bin/client -s 127.0.0.1:8443 -p websocket -anonymous

# KCP
./bin/client -s 127.0.0.1:8000 -p kcp -anonymous

# QUIC
./bin/client -s 127.0.0.1:443 -p quic -anonymous
```

### 3. 压缩加密测试

在生成连接码时启用压缩和加密，验证数据正确传输。

### 4. 断线重连测试

1. 建立隧道连接
2. 断开客户端网络
3. 恢复网络
4. 验证自动重连

## 验证命令

```bash
# 检查进程
ps aux | grep tunnox

# 检查端口
lsof -i :8000
lsof -i :8443

# 查看日志
tail -f ~/GolandProjects/tunnox-core/cmd/server/logs/server.log
```

## 清理

```bash
# 停止所有进程
pkill -f tunnox-server
pkill -f tunnox-client
```

## 测试检查清单

- [ ] 服务端启动正常
- [ ] 客户端连接成功
- [ ] 连接码生成和使用
- [ ] TCP 隧道数据传输
- [ ] 压缩功能正常
- [ ] 加密功能正常
- [ ] 断线重连正常
- [ ] 多协议支持
