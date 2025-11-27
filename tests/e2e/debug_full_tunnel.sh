#!/bin/bash
# 调试完整隧道测试的脚本

set -e

echo "=== 清理旧环境 ==="
docker stop $(docker ps -q) 2>/dev/null || true
docker rm $(docker ps -aq) 2>/dev/null || true
docker network prune -f

cd "$(dirname "$0")"

echo "=== 启动Docker Compose环境 ==="
docker-compose -f docker-compose.full-tunnel.yml up -d

echo "=== 等待服务启动 (60秒) ==="
sleep 60

echo "=== 检查服务状态 ==="
docker-compose -f docker-compose.full-tunnel.yml ps

echo "=== 检查Client-A日志 ==="
echo "--- Last 50 lines of client-a ---"
docker logs tunnox-e2e-$(date +%s 2>/dev/null | head -c 10)*-client-a-1 2>&1 | tail -50 || \
docker logs $(docker ps --filter "name=client-a" --format "{{.ID}}" | head -1) 2>&1 | tail -50

echo ""
echo "=== 检查Client-B日志 ==="
echo "--- Last 50 lines of client-b ---"
docker logs tunnox-e2e-$(date +%s 2>/dev/null | head -c 10)*-client-b-1 2>&1 | tail -50 || \
docker logs $(docker ps --filter "name=client-b" --format "{{.ID}}" | head -1) 2>&1 | tail -50

echo ""
echo "=== 通过API创建映射 ==="
# 等待客户端连接
sleep 5

# 获取在线客户端列表
echo "获取客户端列表..."
CLIENTS=$(curl -s http://localhost:19000/api/v1/clients)
echo "Clients: $CLIENTS"

# 解析客户端ID（假设有两个在线的匿名客户端）
CLIENT_A_ID=$(echo "$CLIENTS" | jq -r '.data[] | select(.status=="online" and .type=="anonymous") | .id' | head -1)
CLIENT_B_ID=$(echo "$CLIENTS" | jq -r '.data[] | select(.status=="online" and .type=="anonymous") | .id' | tail -1)

echo "Client A ID: $CLIENT_A_ID"
echo "Client B ID: $CLIENT_B_ID"

if [ -z "$CLIENT_A_ID" ] || [ -z "$CLIENT_B_ID" ]; then
    echo "❌ 客户端未连接，请检查日志"
    exit 1
fi

# 创建用户
echo "创建用户..."
USER_RESP=$(curl -s -X POST http://localhost:19000/api/v1/users \
    -H "Content-Type: application/json" \
    -d '{"username":"e2e-test","password":"test123","email":"e2e@test.com"}')
echo "User response: $USER_RESP"
USER_ID=$(echo "$USER_RESP" | jq -r '.data.id')
echo "User ID: $USER_ID"

# 创建映射
echo "创建映射..."
MAPPING_RESP=$(curl -s -X POST http://localhost:19000/api/v1/mappings \
    -H "Content-Type: application/json" \
    -d "{
        \"user_id\": \"$USER_ID\",
        \"source_client_id\": $CLIENT_A_ID,
        \"target_client_id\": $CLIENT_B_ID,
        \"protocol\": \"tcp\",
        \"source_port\": 8080,
        \"target_host\": \"target-nginx\",
        \"target_port\": 80,
        \"mapping_name\": \"debug-test\"
    }")
echo "Mapping response: $MAPPING_RESP"

echo ""
echo "=== 等待配置推送 (20秒) ==="
sleep 20

echo ""
echo "=== 再次检查Client-A日志（应该看到ConfigSet） ==="
docker logs $(docker ps --filter "name=client-a" --format "{{.ID}}" | head -1) 2>&1 | grep -E "(ConfigSet|mapping|LocalPort)" | tail -20

echo ""
echo "=== 再次检查Client-B日志（应该看到ConfigSet） ==="
docker logs $(docker ps --filter "name=client-b" --format "{{.ID}}" | head -1) 2>&1 | grep -E "(ConfigSet|mapping|LocalPort)" | tail -20

echo ""
echo "=== 测试端口映射 ==="
for i in {1..5}; do
    echo "尝试 $i/5: 通过 localhost:18080 访问..."
    if curl -s -m 5 http://localhost:18080/ | head -10; then
        echo "✅ 成功！"
        break
    else
        echo "❌ 失败，等待3秒..."
        sleep 3
    fi
done

echo ""
echo "=== 环境保留，可以手动检查 ==="
echo "使用以下命令清理："
echo "  docker-compose -f docker-compose.full-tunnel.yml down"

