#!/bin/bash
set -e

echo "🧪 开始诊断测试..."

# 1. 启动环境
echo "📦 启动Docker Compose环境..."
docker-compose -f docker-compose.full-tunnel.yml up -d

# 2. 等待服务就绪
echo "⏳ 等待服务就绪..."
sleep 30

# 3. 检查客户端
echo "👥 检查在线客户端..."
curl -s http://localhost:19000/api/v1/clients | jq '.data[] | select(.status=="online") | {id, name, status, node_id}'

# 4. 创建用户
echo "👤 创建用户..."
USER_RESP=$(curl -s -X POST http://localhost:19000/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{"username":"test","email":"test@test.com"}')
echo "User response: $USER_RESP"
USER_ID=$(echo $USER_RESP | jq -r '.data.id')
echo "✅ User ID: $USER_ID"

# 5. 获取客户端ID
CLIENT_A_ID=$(curl -s http://localhost:19000/api/v1/clients | jq -r '.data[] | select(.status=="online" and .type=="anonymous") | .id' | head -1)
CLIENT_B_ID=$(curl -s http://localhost:19000/api/v1/clients | jq -r '.data[] | select(.status=="online" and .type=="anonymous") | .id' | tail -1)

echo "✅ Client A ID: $CLIENT_A_ID"
echo "✅ Client B ID: $CLIENT_B_ID"

# 6. 创建Mapping
echo "🔗 创建端口映射..."
MAPPING_RESP=$(curl -s -X POST http://localhost:19000/api/v1/mappings \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\": \"$USER_ID\",
    \"source_client_id\": $CLIENT_A_ID,
    \"target_client_id\": $CLIENT_B_ID,
    \"protocol\": \"tcp\",
    \"source_port\": 8080,
    \"target_port\": 80,
    \"target_host\": \"target-nginx\"
  }")

echo "Mapping response:"
echo "$MAPPING_RESP" | jq '.'

# 7. 检查日志
echo "📜 检查服务器日志（最后50行）..."
docker logs e2e-tunnox-server-1 2>&1 | tail -50

echo "✅ 诊断完成"

