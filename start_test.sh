#!/bin/bash

# 不使用 set -e，避免在后台进程检查时意外退出
# set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 解析参数
START_TARGET_CLIENT=true
START_LISTEN_CLIENT=true

if [ "$1" == "listenclient-only" ] || [ "$1" == "listen-only" ] || [ "$1" == "-l" ]; then
    START_TARGET_CLIENT=false
    echo -e "${YELLOW}Mode: Listen client only (no target client)${NC}"
elif [ "$1" == "targetclient-only" ] || [ "$1" == "target-only" ] || [ "$1" == "-t" ]; then
    START_LISTEN_CLIENT=false
    echo -e "${YELLOW}Mode: Target client only (no listen client)${NC}"
elif [ "$1" == "server-only" ] || [ "$1" == "-s" ]; then
    START_TARGET_CLIENT=false
    START_LISTEN_CLIENT=false
    echo -e "${YELLOW}Mode: Server only${NC}"
fi

echo -e "${YELLOW}=== Starting Tunnox Test Environment ===${NC}"

# 1. 清理所有 server/client 进程（精确匹配，避免误杀）
echo -e "${YELLOW}Step 1: Cleaning up existing processes...${NC}"
# 方法1: 通过进程名称和用户匹配（只匹配 roger.tong 用户的 server/client 进程）
# 使用循环逐个kill，避免xargs可能的问题
ps aux | awk '$1=="roger.tong" && ($11=="server" || $11=="client" || $11=="./server" || $11=="./client" || $11 ~ /\/server$/ || $11 ~ /\/client$/)' | awk '{print $2}' | while read pid; do
    if [ -n "$pid" ] && [ "$pid" != "PID" ]; then
        kill -9 $pid 2>/dev/null || true
    fi
done
# 方法2: 通过路径匹配（更精确，但可能遗漏一些）
ps aux | grep -E "(cmd/server/server|tunnox-core/cmd/server/server|tunnox-core/client|docs/client)" | grep -v grep | awk '{print $2}' | while read pid; do
    if [ -n "$pid" ] && [ "$pid" != "PID" ]; then
        kill -9 $pid 2>/dev/null || true
    fi
done
sleep 2
# 再次确认清理，使用更强制的方式
REMAINING=$(ps aux | awk '$1=="roger.tong" && ($11=="server" || $11=="client" || $11=="./server" || $11=="./client" || $11 ~ /\/server$/ || $11 ~ /\/client$/)' | wc -l | tr -d ' ')
if [ "$REMAINING" -gt 0 ]; then
    echo -e "${YELLOW}Warning: $REMAINING processes still running, attempting final cleanup...${NC}"
    ps aux | awk '$1=="roger.tong" && ($11=="server" || $11=="client" || $11=="./server" || $11=="./client" || $11 ~ /\/server$/ || $11 ~ /\/client$/)' | awk '{print $2}' | while read pid; do
        if [ -n "$pid" ] && [ "$pid" != "PID" ]; then
            # 尝试多种方式kill
            kill -9 $pid 2>/dev/null || kill -TERM $pid 2>/dev/null || true
        fi
    done
    sleep 1
    # 最后检查，如果还有残留，可能是zombie进程，可以忽略
    FINAL_REMAINING=$(ps aux | awk '$1=="roger.tong" && ($11=="server" || $11=="client" || $11=="./server" || $11=="./client" || $11 ~ /\/server$/ || $11 ~ /\/client$/)' | awk '$8!="Z"' | wc -l | tr -d ' ')
    if [ "$FINAL_REMAINING" -gt 0 ]; then
        echo -e "${YELLOW}Note: $FINAL_REMAINING processes may be zombie or unkillable${NC}"
    fi
fi
echo -e "${GREEN}✓ Processes cleaned${NC}"

# 1.5. 清理日志文件
echo -e "${YELLOW}Step 1.5: Cleaning up log files...${NC}"

# 定义配置文件路径数组
CONFIG_FILES=(
    "/Users/roger.tong/GolandProjects/tunnox-core/cmd/server/config.yaml:Server"
    "/Users/roger.tong/GolandProjects/tunnox-core/client-config.yaml:Target Client"
    "/Users/roger.tong/GolandProjects/docs/client-config.yaml:Listen Client"
)

# 函数：从 YAML 配置文件中提取日志文件路径
extract_log_path() {
    local config_file="$1"
    if [ ! -f "$config_file" ]; then
        return
    fi

    # 使用 grep + awk 解析 YAML 中的 log.file 路径
    # 查找 "file:" 开头的行，提取路径（去除引号和空格）
    grep -A 5 "^log:" "$config_file" | grep "file:" | head -1 | awk -F: '{print $2}' | tr -d ' "' | tr -d "'"
}

# 清理从配置文件中读取的日志路径
LOG_COUNT=0
for entry in "${CONFIG_FILES[@]}"; do
    # 分割配置文件路径和描述
    IFS=':' read -r config_file description <<< "$entry"

    # 提取日志路径
    log_path=$(extract_log_path "$config_file")

    if [ -n "$log_path" ]; then
        if [ -f "$log_path" ]; then
            rm -f "$log_path"
            echo -e "${GREEN}  ✓ Removed $description log: $log_path${NC}"
            LOG_COUNT=$((LOG_COUNT + 1))
        else
            echo -e "  ○ $description log not found: $log_path"
        fi
    else
        echo -e "${YELLOW}  ! Could not extract log path from $description config${NC}"
    fi
done

# 额外清理：删除可能存在的其他日志文件
# 1. Server 的 logs 目录
if [ -d "/Users/roger.tong/GolandProjects/tunnox-core/cmd/server/logs" ]; then
    rm -rf /Users/roger.tong/GolandProjects/tunnox-core/cmd/server/logs/*.log 2>/dev/null || true
    echo -e "${GREEN}  ✓ Cleaned server logs directory${NC}"
fi

# 2. /tmp 下的 tunnox-*.log
for tmp_log in /tmp/tunnox-*.log; do
    if [ -f "$tmp_log" ]; then
        rm -f "$tmp_log"
        echo -e "${GREEN}  ✓ Removed temporary log: $tmp_log${NC}"
        LOG_COUNT=$((LOG_COUNT + 1))
    fi
done

echo -e "${GREEN}✓ Log files cleaned (total: $LOG_COUNT files)${NC}"

# 2. 编译 server
echo -e "${YELLOW}Step 2: Building server...${NC}"
cd /Users/roger.tong/GolandProjects/tunnox-core
go build -o bin/server ./cmd/server
if [ ! -f bin/server ]; then
    echo -e "${RED}✗ Server build failed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Server built${NC}"

# 3. Copy server 到指定目录
echo -e "${YELLOW}Step 3: Copying server to /Users/roger.tong/GolandProjects/tunnox-core/cmd/server...${NC}"
cp bin/server /Users/roger.tong/GolandProjects/tunnox-core/cmd/server/server
echo -e "${GREEN}✓ Server copied${NC}"

# 4. 启动 server
echo -e "${YELLOW}Step 4: Starting server...${NC}"
cd /Users/roger.tong/GolandProjects/tunnox-core/cmd/server
./server > /tmp/server_startup.log 2>&1 < /dev/null &
SERVER_PID=$!
sleep 3
if ! ps -p $SERVER_PID > /dev/null 2>&1; then
    echo -e "${RED}✗ Server failed to start${NC}"
    echo "Server startup log:"
    cat /tmp/server_startup.log 2>/dev/null || echo "No startup log"
    exit 1
fi
echo -e "${GREEN}✓ Server started (PID: $SERVER_PID)${NC}"

# 5. 编译 client
echo -e "${YELLOW}Step 5: Building client...${NC}"
cd /Users/roger.tong/GolandProjects/tunnox-core
go build -o bin/client ./cmd/client
if [ ! -f bin/client ]; then
    echo -e "${RED}✗ Client build failed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Client built${NC}"

# 6. Copy client 到 targetclient 目录（如果需要）
if [ "$START_TARGET_CLIENT" = true ]; then
echo -e "${YELLOW}Step 6: Copying client to /Users/roger.tong/GolandProjects/tunnox-core (targetclient)...${NC}"
cp bin/client /Users/roger.tong/GolandProjects/tunnox-core/client
echo -e "${GREEN}✓ Client copied for targetclient${NC}"

# 7. 启动 targetclient（启用调试 API）
echo -e "${YELLOW}Step 7: Starting targetclient (with debug API)...${NC}"
cd /Users/roger.tong/GolandProjects/tunnox-core
# 使用配置的日志路径，不重定向（客户端会自己处理日志）
# 启用调试 API，端口 18081
./client -daemon -debug-api -debug-api-port 18081 < /dev/null > /dev/null 2>&1 &
TARGET_CLIENT_PID=$!
sleep 3
if ! ps -p $TARGET_CLIENT_PID > /dev/null 2>&1; then
    echo -e "${RED}✗ Target client failed to start${NC}"
    echo "Target client log:"
    tail -20 /tmp/tunnox-target-client.log 2>/dev/null || echo "No log"
    exit 1
fi
echo -e "${GREEN}✓ Target client started (PID: $TARGET_CLIENT_PID, Debug API: http://127.0.0.1:18081)${NC}"
else
    echo -e "${YELLOW}Step 6-7: Skipping target client${NC}"
    TARGET_CLIENT_PID=""
fi

# 8. Copy client 到 listenclient 目录（如果需要）
if [ "$START_LISTEN_CLIENT" = true ]; then
echo -e "${YELLOW}Step 8: Copying client to /Users/roger.tong/GolandProjects/docs (listenclient)...${NC}"
cp bin/client /Users/roger.tong/GolandProjects/docs/client
echo -e "${GREEN}✓ Client copied for listenclient${NC}"

# 9. 启动 listenclient
echo -e "${YELLOW}Step 9: Starting listenclient...${NC}"
cd /Users/roger.tong/GolandProjects/docs
# 使用配置的日志路径，不重定向（客户端会自己处理日志）
./client -daemon < /dev/null > /dev/null 2>&1 &
LISTEN_CLIENT_PID=$!
sleep 3
if ! ps -p $LISTEN_CLIENT_PID > /dev/null 2>&1; then
    echo -e "${RED}✗ Listen client failed to start${NC}"
    echo "Listen client log:"
    tail -20 /tmp/tunnox-listen-client.log 2>/dev/null || echo "No log"
    exit 1
fi
echo -e "${GREEN}✓ Listen client started (PID: $LISTEN_CLIENT_PID)${NC}"
else
    echo -e "${YELLOW}Step 8-9: Skipping listen client${NC}"
    LISTEN_CLIENT_PID=""
fi

# 启动完成
    echo -e "${GREEN}=== All services started successfully ===${NC}"
    echo ""
    echo "Server PID: $SERVER_PID"
if [ "$START_TARGET_CLIENT" = true ]; then
    echo "Target Client PID: $TARGET_CLIENT_PID"
fi
if [ "$START_LISTEN_CLIENT" = true ]; then
    echo "Listen Client PID: $LISTEN_CLIENT_PID"
fi
    echo ""
    echo "Logs:"
    echo "  Server: /Users/roger.tong/GolandProjects/tunnox-core/cmd/server/logs/server.log"
if [ "$START_TARGET_CLIENT" = true ]; then
    echo "  Target Client: /tmp/tunnox-target-client.log"
fi
if [ "$START_LISTEN_CLIENT" = true ]; then
    echo "  Listen Client: /tmp/tunnox-listen-client.log"
fi
    echo ""
if [ "$START_TARGET_CLIENT" = true ]; then
    echo "Debug API:"
    echo "  Target Client: http://127.0.0.1:18081/api/v1/status"
fi
    exit 0

