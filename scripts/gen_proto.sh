#!/bin/bash

# 生成 gRPC Go 代码的脚本

set -e

# 项目根目录
PROJECT_ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$PROJECT_ROOT"

echo "Generating gRPC code from proto files..."

# 检查 protoc 是否安装
if ! command -v protoc &> /dev/null; then
    echo "Error: protoc is not installed. Please install Protocol Buffers compiler."
    echo "Visit: https://grpc.io/docs/protoc-installation/"
    exit 1
fi

# 检查 protoc-gen-go 是否安装
if ! command -v protoc-gen-go &> /dev/null; then
    echo "Error: protoc-gen-go is not installed."
    echo "Install with: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
    exit 1
fi

# 检查 protoc-gen-go-grpc 是否安装
if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "Error: protoc-gen-go-grpc is not installed."
    echo "Install with: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
    exit 1
fi

# 生成 bridge.proto
echo "Generating bridge service..."
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    api/proto/bridge/bridge.proto

echo "gRPC code generation completed successfully!"

