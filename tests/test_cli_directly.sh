#!/bin/bash

# 直接测试CLI（使用script命令模拟TTY）

echo "🧪 测试 Tunnox CLI（使用TTY模拟）"
echo ""

CLI_BIN="./bin/tunnox-client"

if [ ! -f "$CLI_BIN" ]; then
    echo "❌ 客户端不存在"
    exit 1
fi

echo "📋 方法1: 使用 script 命令模拟 TTY"
echo ""

# macOS上使用script
if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "在macOS上，请手动运行:"
    echo "  $CLI_BIN"
    echo ""
    echo "或者使用expect:"
    echo "  expect -c 'spawn $CLI_BIN; interact'"
else
    # Linux
    script -q -c "$CLI_BIN" /dev/null
fi

echo ""
echo "📋 方法2: 直接运行（推荐）"
echo "请直接在终端中运行:"
echo "  $CLI_BIN"

