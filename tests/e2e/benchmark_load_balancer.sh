#!/bin/bash

set -e

echo "🚀 Starting Load Balancer Performance Benchmark..."
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "Project root: $PROJECT_ROOT"
echo "Script directory: $SCRIPT_DIR"
echo ""

# 切换到项目根目录
cd "$PROJECT_ROOT"

# 清理函数
cleanup() {
    echo ""
    echo "🧹 Cleaning up..."
    cd "$SCRIPT_DIR"
    docker-compose -f docker-compose.load-balancer.yml down -v
    echo "✅ Cleanup complete"
}

# 注册清理函数
trap cleanup EXIT

# 构建Server镜像
echo "📦 Building Tunnox Server image..."
docker build -f tests/e2e/Dockerfile.server -t tunnox-server:test . || {
    echo -e "${RED}❌ Failed to build server image${NC}"
    exit 1
}
echo -e "${GREEN}✅ Server image built${NC}"
echo ""

# 启动环境
echo "🐳 Starting Docker Compose environment..."
cd "$SCRIPT_DIR"
docker-compose -f docker-compose.load-balancer.yml up -d || {
    echo -e "${RED}❌ Failed to start Docker Compose${NC}"
    exit 1
}
echo ""

# 等待服务就绪
echo "⏳ Waiting for services to be ready..."
sleep 10

# 检查服务健康状态
check_health() {
    local service=$1
    local max_attempts=30
    local attempt=0

    while [ $attempt -lt $max_attempts ]; do
        if docker-compose -f docker-compose.load-balancer.yml ps | grep "$service" | grep -q "healthy\|Up"; then
            echo -e "${GREEN}✓${NC} $service is ready"
            return 0
        fi
        
        attempt=$((attempt + 1))
        echo -e "${YELLOW}⏳${NC} Waiting for $service... (attempt $attempt/$max_attempts)"
        sleep 2
    done

    echo -e "${RED}❌${NC} $service failed to become healthy"
    return 1
}

# 检查所有关键服务
check_health "redis"
check_health "tunnox-server-1"
check_health "tunnox-server-2"
check_health "tunnox-server-3"
check_health "nginx"

echo ""
echo -e "${GREEN}✅ All services are ready${NC}"
echo ""

# 性能基准测试报告文件
REPORT_FILE="$SCRIPT_DIR/benchmark_report_$(date +%Y%m%d_%H%M%S).txt"

echo "📊 Performance Benchmark Report" > "$REPORT_FILE"
echo "=================================" >> "$REPORT_FILE"
echo "Date: $(date)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# 测试1: 健康检查性能
echo "📊 Test 1: Health Check Performance"
echo "" >> "$REPORT_FILE"
echo "Test 1: Health Check Performance" >> "$REPORT_FILE"
echo "---------------------------------" >> "$REPORT_FILE"

health_check_count=100
success_count=0
start_time=$(date +%s.%N)

for i in $(seq 1 $health_check_count); do
    if curl -f -s http://localhost:8080/health > /dev/null 2>&1; then
        success_count=$((success_count + 1))
    fi
done

end_time=$(date +%s.%N)
duration=$(echo "$end_time - $start_time" | bc)
qps=$(echo "scale=2; $health_check_count / $duration" | bc)
success_rate=$(echo "scale=2; $success_count * 100 / $health_check_count" | bc)

echo "  Total requests: $health_check_count" | tee -a "$REPORT_FILE"
echo "  Success: $success_count" | tee -a "$REPORT_FILE"
echo "  Success rate: $success_rate%" | tee -a "$REPORT_FILE"
echo "  Duration: ${duration}s" | tee -a "$REPORT_FILE"
echo "  QPS: $qps" | tee -a "$REPORT_FILE"
echo "" | tee -a "$REPORT_FILE"

if (( $(echo "$success_rate >= 95" | bc -l) )); then
    echo -e "${GREEN}✅ Health check performance: PASSED${NC}"
else
    echo -e "${RED}❌ Health check performance: FAILED${NC}"
fi
echo ""

# 测试2: 并发请求性能
echo "📊 Test 2: Concurrent Request Performance"
echo "" >> "$REPORT_FILE"
echo "Test 2: Concurrent Request Performance" >> "$REPORT_FILE"
echo "---------------------------------------" >> "$REPORT_FILE"

# 使用GNU parallel进行并发测试（如果可用）
if command -v parallel &> /dev/null; then
    echo "  Using GNU parallel for concurrent testing..."
    
    start_time=$(date +%s.%N)
    seq 1 100 | parallel -j 20 "curl -f -s http://localhost:8080/health > /dev/null 2>&1" 
    end_time=$(date +%s.%N)
    
    duration=$(echo "$end_time - $start_time" | bc)
    qps=$(echo "scale=2; 100 / $duration" | bc)
    
    echo "  Concurrent requests: 100 (20 workers)" | tee -a "$REPORT_FILE"
    echo "  Duration: ${duration}s" | tee -a "$REPORT_FILE"
    echo "  QPS: $qps" | tee -a "$REPORT_FILE"
    echo "" | tee -a "$REPORT_FILE"
    
    echo -e "${GREEN}✅ Concurrent request test completed${NC}"
else
    echo -e "${YELLOW}⚠️  GNU parallel not found, skipping concurrent test${NC}"
    echo "  Skipped: GNU parallel not available" >> "$REPORT_FILE"
fi
echo ""

# 测试3: 故障转移性能
echo "📊 Test 3: Failover Performance"
echo "" >> "$REPORT_FILE"
echo "Test 3: Failover Performance" >> "$REPORT_FILE"
echo "----------------------------" >> "$REPORT_FILE"

# 停止Server-1
echo "  Stopping tunnox-server-1..."
docker-compose -f docker-compose.load-balancer.yml stop tunnox-server-1

# 等待Nginx检测到故障
sleep 5

# 测试故障转移后的性能
failover_count=50
failover_success=0
start_time=$(date +%s.%N)

for i in $(seq 1 $failover_count); do
    if curl -f -s http://localhost:8080/health > /dev/null 2>&1; then
        failover_success=$((failover_success + 1))
    fi
    sleep 0.1
done

end_time=$(date +%s.%N)
duration=$(echo "$end_time - $start_time" | bc)
failover_rate=$(echo "scale=2; $failover_success * 100 / $failover_count" | bc)

echo "  Requests after failover: $failover_count" | tee -a "$REPORT_FILE"
echo "  Success: $failover_success" | tee -a "$REPORT_FILE"
echo "  Success rate: $failover_rate%" | tee -a "$REPORT_FILE"
echo "  Duration: ${duration}s" | tee -a "$REPORT_FILE"
echo "" | tee -a "$REPORT_FILE"

if (( $(echo "$failover_rate >= 80" | bc -l) )); then
    echo -e "${GREEN}✅ Failover performance: PASSED${NC}"
else
    echo -e "${RED}❌ Failover performance: FAILED${NC}"
fi

# 重启Server-1
echo "  Restarting tunnox-server-1..."
docker-compose -f docker-compose.load-balancer.yml start tunnox-server-1
sleep 10

echo ""

# 测试4: 负载分布
echo "📊 Test 4: Load Distribution"
echo "" >> "$REPORT_FILE"
echo "Test 4: Load Distribution" >> "$REPORT_FILE"
echo "-------------------------" >> "$REPORT_FILE"

echo "  Checking request distribution across servers..."
echo "  (This is a placeholder - real distribution check requires application logs)" | tee -a "$REPORT_FILE"
echo "" | tee -a "$REPORT_FILE"

# 测试5: 资源使用
echo "📊 Test 5: Resource Usage"
echo "" >> "$REPORT_FILE"
echo "Test 5: Resource Usage" >> "$REPORT_FILE"
echo "----------------------" >> "$REPORT_FILE"

echo "  Container resource usage:" | tee -a "$REPORT_FILE"
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" \
    | grep -E "tunnox-server|redis|nginx" | tee -a "$REPORT_FILE"
echo "" | tee -a "$REPORT_FILE"

# 完成
echo ""
echo "================================="
echo "📊 Benchmark Summary"
echo "================================="
echo ""
echo "Report saved to: $REPORT_FILE"
echo ""

# 显示总结
cat "$REPORT_FILE"

echo ""
echo -e "${GREEN}✅ Benchmark complete${NC}"
echo ""

