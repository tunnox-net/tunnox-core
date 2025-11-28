#!/bin/bash

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Tunnox 核心功能 E2E 测试运行脚本
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 项目根目录
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO_BIN="/Users/roger.tong/sdk/go1.24.4/bin/go"

echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${CYAN}  Tunnox 核心功能 E2E 测试${NC}"
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

# 解析参数
RUN_MODE="quick"  # quick | full | specific
SPECIFIC_TEST=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --full)
            RUN_MODE="full"
            shift
            ;;
        --test)
            RUN_MODE="specific"
            SPECIFIC_TEST="$2"
            shift 2
            ;;
        --help)
            echo "使用方法:"
            echo "  $0                    # 运行快速测试（跳过慢速测试）"
            echo "  $0 --full             # 运行完整测试（包含Docker环境）"
            echo "  $0 --test TestName    # 运行特定测试"
            echo "  $0 --help             # 显示帮助"
            exit 0
            ;;
        *)
            log_error "未知参数: $1"
            exit 1
            ;;
    esac
done

cd "$PROJECT_ROOT"

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 模式1: 快速测试（不需要Docker）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

if [ "$RUN_MODE" == "quick" ]; then
    log_info "运行快速测试模式（-short）..."
    echo ""
    
    log_info "🧪 运行所有单元测试..."
    if "$GO_BIN" test -short -v ./tests/e2e/... 2>&1 | grep -E "(PASS|FAIL|SKIP|===)"; then
        log_success "快速测试完成"
    else
        log_error "快速测试失败"
        exit 1
    fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 模式2: 完整测试（需要Docker）
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

elif [ "$RUN_MODE" == "full" ]; then
    log_warning "运行完整测试模式（需要Docker，耗时30-60分钟）..."
    echo ""
    
    # 检查Docker
    if ! command -v docker &> /dev/null; then
        log_error "Docker未安装，无法运行完整测试"
        exit 1
    fi
    
    # 检查docker-compose
    if ! command -v docker-compose &> /dev/null; then
        log_error "docker-compose未安装，无法运行完整测试"
        exit 1
    fi
    
    log_info "🏗️  构建测试镜像..."
    cd tests/e2e
    if docker build -f Dockerfile.server -t tunnox-server:test ../.. 2>&1 | tail -5; then
        log_success "镜像构建完成"
    else
        log_error "镜像构建失败"
        exit 1
    fi
    
    cd "$PROJECT_ROOT"
    
    log_info "🧪 运行完整E2E测试..."
    if "$GO_BIN" test -v ./tests/e2e/... -timeout 60m 2>&1 | tee /tmp/tunnox-e2e-test.log | grep -E "(PASS|FAIL|RUN|===)"; then
        log_success "完整测试完成"
        echo ""
        log_info "完整日志保存在: /tmp/tunnox-e2e-test.log"
    else
        log_error "完整测试失败"
        exit 1
    fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 模式3: 特定测试
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

elif [ "$RUN_MODE" == "specific" ]; then
    log_info "运行特定测试: $SPECIFIC_TEST"
    echo ""
    
    if "$GO_BIN" test -v ./tests/e2e/... -run "$SPECIFIC_TEST" -timeout 30m 2>&1; then
        log_success "测试完成"
    else
        log_error "测试失败"
        exit 1
    fi
fi

echo ""
log_success "所有测试完成！"
echo ""

