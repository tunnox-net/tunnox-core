#!/bin/bash

# CLI手动测试脚本

echo "🧪 Tunnox CLI 手动测试"
echo ""

CLI_BIN="./bin/tunnox-client"

if [ ! -f "$CLI_BIN" ]; then
    echo "❌ 客户端二进制文件不存在，请先编译"
    exit 1
fi

echo "✅ 找到客户端: $CLI_BIN"
echo ""

echo "📋 测试1: 检查二进制文件"
file "$CLI_BIN"
echo ""

echo "📋 测试2: 检查终端"
if [ -t 0 ]; then
    echo "✅ stdin 是 TTY (交互式终端)"
else
    echo "⚠️  stdin 不是 TTY (可能通过管道/重定向)"
fi

if [ -t 1 ]; then
    echo "✅ stdout 是 TTY"
else
    echo "⚠️  stdout 不是 TTY"
fi
echo ""

echo "📋 测试3: 启动客户端（10秒后自动退出）"
echo "请在10秒内测试CLI功能..."
echo ""

timeout 10 "$CLI_BIN" || true

echo ""
echo "✅ 测试完成"

