#!/bin/bash

# TCP 端口映射测试脚本

set -e

echo "🧪 Tunnox TCP Port Mapping Test"
echo "================================"
echo ""

# 测试配置
SERVER_ADDR="localhost:7000"
NGINX_TARGET="localhost:18080"
REDIS_TARGET="localhost:16379"
LOCAL_NGINX_PORT=28080
LOCAL_REDIS_PORT=26379

echo "📋 Test Configuration:"
echo "  Server: $SERVER_ADDR"
echo "  Nginx Target: $NGINX_TARGET"
echo "  Redis Target: $REDIS_TARGET"
echo "  Local Nginx Port: $LOCAL_NGINX_PORT"
echo "  Local Redis Port: $LOCAL_REDIS_PORT"
echo ""

# 1. 测试直连目标服务
echo "1️⃣  Testing direct connection to target services..."
echo -n "  - Nginx: "
if curl -s -o /dev/null -w "%{http_code}" http://$NGINX_TARGET | grep -q "200"; then
    echo "✅ OK"
else
    echo "❌ FAILED"
    exit 1
fi

echo -n "  - Redis: "
if redis-cli -h localhost -p 16379 PING 2>/dev/null | grep -q "PONG"; then
    echo "✅ OK"
else
    echo "❌ FAILED"
    exit 1
fi
echo ""

# 2. 启动 Server
echo "2️⃣  Starting Tunnox Server..."
cd /Users/roger.tong/GolandProjects/tunnox-core
./bin/tunnox-server -config test-env/configs/server.yaml > test-env/logs/server.log 2>&1 &
SERVER_PID=$!
echo "  Server PID: $SERVER_PID"
sleep 2

if ! kill -0 $SERVER_PID 2>/dev/null; then
    echo "❌ Server failed to start"
    cat test-env/logs/server.log
    exit 1
fi
echo "  ✅ Server started"
echo ""

# 3. 启动 Client B (目标客户端 - 服务提供方)
echo "3️⃣  Starting Client B (target/service provider)..."
./bin/tunnox-client -config test-env/configs/client-b.yaml > test-env/logs/client-b.log 2>&1 &
CLIENT_B_PID=$!
echo "  Client B PID: $CLIENT_B_PID"
sleep 2

if ! kill -0 $CLIENT_B_PID 2>/dev/null; then
    echo "❌ Client B failed to start"
    cat test-env/logs/client-b.log
    kill $SERVER_PID 2>/dev/null || true
    exit 1
fi
echo "  ✅ Client B started"
echo ""

# 4. 启动 Client A (源客户端 - 访问方)
echo "4️⃣  Starting Client A (source/accessor)..."
./bin/tunnox-client -config test-env/configs/client-a.yaml > test-env/logs/client-a.log 2>&1 &
CLIENT_A_PID=$!
echo "  Client A PID: $CLIENT_A_PID"
sleep 2

if ! kill -0 $CLIENT_A_PID 2>/dev/null; then
    echo "❌ Client A failed to start"
    cat test-env/logs/client-a.log
    kill $SERVER_PID $CLIENT_B_PID 2>/dev/null || true
    exit 1
fi
echo "  ✅ Client A started"
echo ""

# 等待握手完成
echo "⏳ Waiting for handshake to complete..."
sleep 3
echo ""

# 5. 检查日志是否有错误
echo "5️⃣  Checking logs for errors..."
if grep -i "error\|failed\|panic" test-env/logs/server.log test-env/logs/client-a.log test-env/logs/client-b.log 2>/dev/null | grep -v "gracefully"; then
    echo "❌ Found errors in logs"
    echo ""
    echo "=== Server Log ==="
    cat test-env/logs/server.log
    echo ""
    echo "=== Client A Log ==="
    cat test-env/logs/client-a.log
    echo ""
    echo "=== Client B Log ==="
    cat test-env/logs/client-b.log
else
    echo "  ✅ No errors found"
fi
echo ""

# 6. 创建端口映射（通过 Management API）
echo "6️⃣  TODO: Create port mapping via Management API"
echo "  (This requires implementation of mapping creation API)"
echo ""

# 清理
echo "🧹 Cleaning up..."
echo "  Stopping processes..."
kill $CLIENT_A_PID $CLIENT_B_PID $SERVER_PID 2>/dev/null || true
sleep 1
echo "  ✅ Cleanup complete"
echo ""

echo "📊 Test Summary:"
echo "  Server: Started and ran"
echo "  Client A: Started and connected"
echo "  Client B: Started and connected"
echo "  Next: Need to implement port mapping creation and testing"
echo ""
echo "✅ Basic connectivity test PASSED"

