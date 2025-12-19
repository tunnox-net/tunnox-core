.PHONY: help test quick-test install clean

help:
	@echo "Tunnox UDP 集成测试"
	@echo ""
	@echo "可用命令:"
	@echo "  make install      - 安装 Python 依赖"
	@echo "  make test         - 运行完整集成测试"
	@echo "  make quick-test   - 运行快速测试（仅测试连接）"
	@echo "  make clean        - 清理测试环境"
	@echo "  make logs         - 查看日志"
	@echo ""

install:
	@echo "安装 Python 依赖..."
	pip3 install pymysql pyyaml

test:
	@echo "运行完整集成测试..."
	./integration_test.py

quick-test:
	@echo "运行快速测试..."
	./quick_test.py

clean:
	@echo "清理测试环境..."
	@pkill -f 'tunnox.*client' || true
	@rm -rf ~/tunnox-test/listen-client/logs
	@rm -rf ~/tunnox-test/target-client/logs
	@echo "清理完成"

logs:
	@echo "=== Listen Client 日志 ==="
	@tail -n 30 ~/tunnox-test/listen-client/logs/client.log 2>/dev/null || echo "日志文件不存在"
	@echo ""
	@echo "=== Target Client 日志 ==="
	@tail -n 30 ~/tunnox-test/target-client/logs/client.log 2>/dev/null || echo "日志文件不存在"

status:
	@echo "=== 进程状态 ==="
	@ps aux | grep -E 'tunnox.*(client|server)' | grep -v grep || echo "没有运行的进程"
	@echo ""
	@echo "=== 端口监听 ==="
	@lsof -i :9988 2>/dev/null || echo "端口 9988 未监听"
