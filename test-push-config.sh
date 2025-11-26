#!/bin/bash

echo "=== 🧪 配置推送完整测试 ==="
echo ""

# 清理旧进程
killall -9 tunnox-server tunnox-client 2>/dev/null
sleep 1

# 1. 启动Server
echo "1️⃣ 启动Server..."
./bin/tunnox-server -config test-env/configs/server.yaml > /tmp/test-server.log 2>&1 &
SERVER_PID=$!
sleep 3

# 2. 启动Client A & B
echo "2️⃣ 启动Client A & B..."
./bin/tunnox-client -config test-env/configs/client-a.yaml > /tmp/test-client-a.log 2>&1 &
CLIENT_A_PID=$!
./bin/tunnox-client -config test-env/configs/client-b.yaml > /tmp/test-client-b.log 2>&1 &
CLIENT_B_PID=$!
sleep 5

echo "📊 Client状态:"
echo "  Client A: $(tail -3 /tmp/test-client-a.log | grep "ClientID=" || echo "未认证")"
echo "  Client B: $(tail -3 /tmp/test-client-b.log | grep "ClientID=" || echo "未认证")"
echo ""

# 3. 创建用户
echo "3️⃣ 创建用户..."
API_KEY="test-api-key-for-management-api-1234567890"
USER_RESP=$(curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "email": "test@example.com"}' \
  http://localhost:9000/api/v1/users)
USER_ID=$(echo $USER_RESP | jq -r '.data.id')
echo "User ID: $USER_ID"

# 4. 创建TCP映射（这会触发配置推送）
echo ""
echo "4️⃣ 创建TCP映射 (Source: ClientA, Target: ClientB)..."
MAPPING_RESP=$(curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\": \"$USER_ID\",
    \"source_client_id\": 10000001,
    \"target_client_id\": 10000002,
    \"protocol\": \"tcp\",
    \"source_port\": 8080,
    \"target_host\": \"localhost\",
    \"target_port\": 80
  }" \
  http://localhost:9000/api/v1/mappings)
MAPPING_ID=$(echo $MAPPING_RESP | jq -r '.data.id')
echo "Mapping ID: $MAPPING_ID"

# 5. 等待配置推送
echo ""
echo "5️⃣ 等待配置推送生效..."
sleep 3

# 6. 检查Client日志
echo ""
echo "=== 📋 Client A 日志（最后10行）==="
tail -10 /tmp/test-client-a.log

echo ""
echo "=== 📋 Client B 日志（最后10行）==="
tail -10 /tmp/test-client-b.log

echo ""
echo "=== 📋 Server 日志（ConfigSet相关）==="
cat /tmp/test-server.log | grep -E "push|ConfigSet|API:" | tail -20

echo ""
echo "✅ 测试完成！"
echo "进程状态："
ps aux | grep "bin/tunnox" | grep -v grep | awk '{print "  PID " $2 ": " $11}'

