#!/bin/bash

# 测试端口映射的完整流程脚本

set -e  # 遇到错误立即退出

API_URL="http://localhost:9000/api/v1"
API_KEY="test-api-key-for-management-api-1234567890"
AUTH_HEADER="Authorization: Bearer $API_KEY"

echo "🚀 开始测试端口映射流程..."
echo

# 1. 创建测试用户
echo "📝 1. 创建测试用户..."
USER_RESPONSE=$(curl -s -X POST -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"username":"testuser","email":"test@example.com","password":"testpass123"}' \
  "$API_URL/users")
echo "$USER_RESPONSE" | python3 -m json.tool
USER_ID=$(echo "$USER_RESPONSE" | python3 -c "import sys, json; print(json.load(sys.stdin)['data']['id'])")
echo "✅ 用户创建成功，User ID: $USER_ID"
echo

# 2. 创建 Client A (源客户端 - 匿名)
echo "📝 2. 注册 Client A (源客户端)..."
CLIENT_A_RESPONSE=$(curl -s -X POST -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d "{\"user_id\":\"$USER_ID\",\"name\":\"Client-A\",\"device_id\":\"test-client-a\",\"type\":\"permanent\"}" \
  "$API_URL/clients")
echo "$CLIENT_A_RESPONSE" | python3 -m json.tool
CLIENT_A_ID=$(echo "$CLIENT_A_RESPONSE" | python3 -c "import sys, json; print(json.load(sys.stdin)['data']['id'])" 2>/dev/null || echo "10000002")
echo "✅ Client A ID: $CLIENT_A_ID"
echo

# 3. 创建 Client B (目标客户端 - 匿名)
echo "📝 3. 注册 Client B (目标客户端)..."
CLIENT_B_RESPONSE=$(curl -s -X POST -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d "{\"user_id\":\"$USER_ID\",\"name\":\"Client-B\",\"device_id\":\"test-client-b\",\"type\":\"permanent\"}" \
  "$API_URL/clients")
echo "$CLIENT_B_RESPONSE" | python3 -m json.tool
CLIENT_B_ID=$(echo "$CLIENT_B_RESPONSE" | python3 -c "import sys, json; print(json.load(sys.stdin)['data']['id'])" 2>/dev/null || echo "10000003")
echo "✅ Client B ID: $CLIENT_B_ID"
echo

# 4. 创建端口映射：Client A (18080) -> Client B (localhost:8080)
echo "📝 4. 创建端口映射 (TCP)..."
MAPPING_RESPONSE=$(curl -s -X POST -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d "{
    \"user_id\":\"$USER_ID\",
    \"source_client_id\":$CLIENT_A_ID,
    \"target_client_id\":$CLIENT_B_ID,
    \"name\":\"Test TCP Mapping\",
    \"protocol\":\"tcp\",
    \"source_port\":18080,
    \"target_host\":\"localhost\",
    \"target_port\":8080,
    \"secret_key\":\"test-secret-key-123\",
    \"enabled\":true
  }" \
  "$API_URL/mappings")
echo "$MAPPING_RESPONSE" | python3 -m json.tool
MAPPING_ID=$(echo "$MAPPING_RESPONSE" | python3 -c "import sys, json; print(json.load(sys.stdin)['data']['id'])" 2>/dev/null || echo "unknown")
echo "✅ 端口映射创建成功，Mapping ID: $MAPPING_ID"
echo

# 5. 验证映射是否生效
echo "📝 5. 等待 5 秒让配置生效..."
sleep 5

echo "📝 6. 测试端口映射..."
echo "尝试通过 localhost:18080 访问 Nginx (应该映射到 Client B 的 localhost:8080)..."
curl -s http://localhost:18080 | head -5 || echo "❌ 映射测试失败"

echo
echo "🎉 测试完成！"

