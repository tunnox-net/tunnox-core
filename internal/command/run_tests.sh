#!/bin/bash

# 命令框架测试运行脚本

echo "=== 运行命令框架单元测试 ==="

# 设置测试环境
export GO111MODULE=on
export CGO_ENABLED=0

# 运行所有测试
echo "运行所有测试..."
go test -v ./...

# 运行基准测试
echo ""
echo "=== 运行基准测试 ==="
go test -bench=. -benchmem ./...

# 运行覆盖率测试
echo ""
echo "=== 运行覆盖率测试 ==="
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

echo ""
echo "=== 测试完成 ==="
echo "覆盖率报告已生成: coverage.html"
echo "覆盖率数据: coverage.out"

# 清理临时文件
# rm -f coverage.out coverage.html 