#!/bin/bash
# 测试 HTTP Long Polling 客户端 CLI

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "测试 HTTP Long Polling 客户端连接和 CLI"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 检查 server 是否运行
if ! pgrep -f "bin/server" > /dev/null; then
    echo "❌ Server 未运行，请先启动 server"
    exit 1
fi

echo "✅ Server 正在运行"
echo ""

# 检查连接
echo "测试连接到 server..."
cd "$(dirname "$0")"

# 启动 client（交互模式）
echo "启动客户端（交互模式）..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "提示：在 CLI 中可以尝试以下命令："
echo "  - status          : 查看连接状态"
echo "  - list-mappings   : 列出所有映射"
echo "  - generate-code   : 生成连接码"
echo "  - help            : 显示帮助"
echo "  - exit            : 退出"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

./bin/client -p httppoll -s http://127.0.0.1:9000 -anonymous

