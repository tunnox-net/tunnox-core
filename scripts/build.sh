#!/bin/bash
# 构建脚本示例 - 将版本号编译进二进制文件

# 读取版本号
VERSION=$(cat VERSION | tr -d '[:space:]')
if [ -z "$VERSION" ]; then
    echo "Error: VERSION file is empty or not found"
    exit 1
fi

# 获取 Git 提交哈希
GIT_COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "")
BUILD_TIME=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

# 构建参数
LDFLAGS="-X 'tunnox-core/internal/version.Version=$VERSION'"
if [ -n "$GIT_COMMIT" ]; then
    LDFLAGS="$LDFLAGS -X 'tunnox-core/internal/version.GitCommit=$GIT_COMMIT'"
fi
LDFLAGS="$LDFLAGS -X 'tunnox-core/internal/version.BuildTime=$BUILD_TIME'"

# 构建标志
BUILD_FLAGS="-ldflags=\"$LDFLAGS\" -trimpath -s -w"
BUILD_FLAGS="$BUILD_FLAGS -tags netgo"
BUILD_FLAGS="$BUILD_FLAGS -extldflags '-static'"

echo "Building with version: $VERSION"
echo "Build flags: $BUILD_FLAGS"

# 构建 server
echo "Building server..."
go build $BUILD_FLAGS -o bin/tunnox-server ./cmd/server

# 构建 client
echo "Building client..."
go build $BUILD_FLAGS -o bin/tunnox-client ./cmd/client

echo "Build completed!"

