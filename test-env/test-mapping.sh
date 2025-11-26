#!/bin/bash

# 端口映射测试脚本

echo "=== 1. 停止所有进程 ==="
pkill -f "tunnox-server"
pkill -f "tunnox-client"
sleep 2

echo "=== 2. 启动服务器 ==="
cd /Users/roger.tong/GolandProjects/tunnox-core
nohup ./bin/tunnox-server -config test-env/configs/server.yaml > /tmp/test-server.log 2>&1 &
sleep 3

echo "=== 3. 启动客户端 ==="
./bin/tunnox-client -config test-env/configs/client-a.yaml > /tmp/test-client-a.log 2>&1 &
./bin/tunnox-client -config test-env/configs/client-b.yaml > /tmp/test-client-b.log 2>&1 &
sleep 3

echo "=== 4. 获取ClientID ==="
CLIENT_A_ID=$(grep "ClientID=" /tmp/test-client-a.log | tail -1 | sed 's/.*ClientID=\([0-9]*\).*/\1/')
CLIENT_B_ID=$(grep "ClientID=" /tmp/test-client-b.log | tail -1 | sed 's/.*ClientID=\([0-9]*\).*/\1/')
echo "Client A ID: $CLIENT_A_ID"
echo "Client B ID: $CLIENT_B_ID"

echo "=== 5. 创建端口映射（启用压缩）==="
MAPPING_ID=$(curl -s -X POST http://127.0.0.1:9000/api/v1/mappings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-api-key-for-management-api-1234567890" \
  -d "{
    \"user_id\": \"user_test_001\",
    \"source_client_id\": $CLIENT_A_ID,
    \"target_client_id\": $CLIENT_B_ID,
    \"protocol\": \"tcp\",
    \"source_port\": 8080,
    \"target_host\": \"127.0.0.1\",
    \"target_port\": 18080,
    \"enable_compression\": true,
    \"compression_level\": 6
  }" | jq -r '.data.id')

echo "Mapping ID: $MAPPING_ID"
sleep 3

echo "=== 6. 测试端口映射 ==="
echo "Testing http://127.0.0.1:8080 ..."
curl -s -m 10 http://127.0.0.1:8080 | head -5

echo ""
echo "=== 测试完成 ==="

