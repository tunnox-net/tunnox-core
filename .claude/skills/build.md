# /build - 构建服务端和客户端

构建 Tunnox Core 的服务端和客户端二进制文件。

## 执行步骤

1. 编译服务端
2. 编译客户端
3. 验证构建产物

## 命令

```bash
# 构建服务端
go build -o bin/server ./cmd/server

# 构建客户端
go build -o bin/client ./cmd/client

# 或使用脚本 (如果有)
./scripts/build.sh
```

## 输出

构建产物输出到 `bin/` 目录:
- `bin/server` - 服务端二进制
- `bin/client` - 客户端二进制

## 检查项

- [ ] 编译无错误
- [ ] 编译无警告
- [ ] 二进制文件大小合理
- [ ] 可正常启动

## 交叉编译

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o bin/server-linux-amd64 ./cmd/server
GOOS=linux GOARCH=amd64 go build -o bin/client-linux-amd64 ./cmd/client

# Windows AMD64
GOOS=windows GOARCH=amd64 go build -o bin/server-windows-amd64.exe ./cmd/server
GOOS=windows GOARCH=amd64 go build -o bin/client-windows-amd64.exe ./cmd/client

# macOS ARM64
GOOS=darwin GOARCH=arm64 go build -o bin/server-darwin-arm64 ./cmd/server
GOOS=darwin GOARCH=arm64 go build -o bin/client-darwin-arm64 ./cmd/client
```
