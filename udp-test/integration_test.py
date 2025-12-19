#!/usr/bin/env python3
"""
Tunnox UDP 集成测试脚本

功能：
1. 初始化环境（停止服务、编译、部署）
2. 启动服务端和客户端
3. 执行 MySQL 连接测试（3次）
4. 生成测试报告
"""

import os
import sys
import time
import yaml
import subprocess
import signal
from pathlib import Path
from datetime import datetime

try:
    import pymysql
except ImportError:
    print("❌ 缺少 pymysql 模块，请安装: pip3 install pymysql")
    sys.exit(1)

# 配置文件路径
CONFIG_FILE = Path(__file__).parent / "config.yaml"
PROJECT_ROOT = Path(__file__).parent.parent

class Colors:
    """终端颜色"""
    HEADER = '\033[95m'
    BLUE = '\033[94m'
    CYAN = '\033[96m'
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    RED = '\033[91m'
    END = '\033[0m'
    BOLD = '\033[1m'

def log_info(msg):
    print(f"{Colors.CYAN}ℹ {msg}{Colors.END}")

def log_success(msg):
    print(f"{Colors.GREEN}✓ {msg}{Colors.END}")

def log_warning(msg):
    print(f"{Colors.YELLOW}⚠ {msg}{Colors.END}")

def log_error(msg):
    print(f"{Colors.RED}✗ {msg}{Colors.END}")

def log_header(msg):
    print(f"\n{Colors.BOLD}{Colors.HEADER}{'='*60}{Colors.END}")
    print(f"{Colors.BOLD}{Colors.HEADER}{msg}{Colors.END}")
    print(f"{Colors.BOLD}{Colors.HEADER}{'='*60}{Colors.END}\n")

def load_config():
    """加载配置文件"""
    log_info(f"加载配置文件: {CONFIG_FILE}")
    with open(CONFIG_FILE, 'r', encoding='utf-8') as f:
        config = yaml.safe_load(f)
    log_success("配置文件加载成功")
    return config

def run_command(cmd, cwd=None, check=True, capture_output=False, timeout=30):
    """执行命令"""
    log_info(f"执行命令: {cmd}")
    try:
        if capture_output:
            result = subprocess.run(
                cmd, 
                shell=True, 
                cwd=cwd, 
                check=check,
                capture_output=True,
                text=True,
                timeout=timeout
            )
            return result.stdout.strip()
        else:
            subprocess.run(cmd, shell=True, cwd=cwd, check=check, timeout=timeout)
            return None
    except subprocess.TimeoutExpired:
        log_error(f"命令执行超时 ({timeout}s): {cmd}")
        raise
    except subprocess.CalledProcessError as e:
        log_error(f"命令执行失败: {cmd}")
        if capture_output:
            log_error(f"错误输出: {e.stderr}")
        raise

def ssh_command(config, cmd, check=True):
    """通过 SSH 执行远程命令"""
    ssh_config = config['server']['ssh']
    ssh_key = Path(__file__).parent / ssh_config['ssh-key']
    
    ssh_cmd = (
        f"ssh -i {ssh_key} "
        f"-o StrictHostKeyChecking=no "
        f"-o UserKnownHostsFile=/dev/null "
        f"-o LogLevel=ERROR "
        f"{ssh_config['user-name']}@{ssh_config['address']} "
        f"'{cmd}'"
    )
    
    return run_command(ssh_cmd, check=check, capture_output=True)

def stop_service_with_timeout(config, timeout=10):
    """尝试优雅停止服务，超时后强制 kill"""
    import threading
    import subprocess
    
    ssh_config = config['server']['ssh']
    ssh_key = Path(__file__).parent / ssh_config['ssh-key']
    
    # 尝试优雅停止
    stop_cmd = (
        f"ssh -i {ssh_key} "
        f"-o StrictHostKeyChecking=no "
        f"-o UserKnownHostsFile=/dev/null "
        f"-o LogLevel=ERROR "
        f"-o ConnectTimeout=5 "
        f"{ssh_config['user-name']}@{ssh_config['address']} "
        f"'systemctl stop tunnox.service'"
    )
    
    log_info(f"尝试优雅停止服务（超时 {timeout}s）...")
    
    try:
        # 使用 timeout 参数
        subprocess.run(
            stop_cmd,
            shell=True,
            timeout=timeout,
            capture_output=True,
            text=True
        )
        log_success("服务已优雅停止")
        return True
    except subprocess.TimeoutExpired:
        log_warning(f"优雅停止超时（{timeout}s），强制 kill...")
        # 强制 kill
        try:
            ssh_command(config, "pkill -9 -f tunnox-server || true", check=False)
            time.sleep(1)
            log_success("服务已强制停止")
            return True
        except Exception as e:
            log_error(f"强制停止失败: {e}")
            return False

def stop_processes(config):
    """停止所有相关进程"""
    log_header("步骤 1: 停止现有进程")
    
    # 停止远程服务器上的服务
    log_info("停止远程服务器上的 tunnox 服务...")
    try:
        stop_service_with_timeout(config, timeout=10)
        
        # 确认是否还有进程在运行
        result = ssh_command(config, "pgrep -f tunnox-server || true", check=False)
        if result:
            log_warning("发现残留进程，再次强制 kill...")
            ssh_command(config, "pkill -9 -f tunnox-server || true", check=False)
            time.sleep(1)
        
        log_success("远程服务器进程已停止")
    except Exception as e:
        log_error(f"停止远程服务失败: {e}")
        raise
    
    # 停止本地客户端进程
    log_info("停止本地客户端进程...")
    try:
        run_command("pkill -f 'tunnox.*client' || true", check=False, timeout=10)
        time.sleep(2)
        
        # 确认是否还有进程在运行
        result = run_command("pgrep -f 'tunnox.*client' || true", check=False, capture_output=True, timeout=5)
        if result:
            log_warning("发现残留客户端进程，强制 kill...")
            run_command("pkill -9 -f 'tunnox.*client' || true", check=False, timeout=5)
            time.sleep(1)
        
        log_success("本地客户端进程已停止")
    except Exception as e:
        log_error(f"停止本地进程失败: {e}")
        raise

def build_binaries():
    """编译 server 和 client"""
    log_header("步骤 2: 编译二进制文件")
    
    # 编译 server (Linux amd64)
    log_info("编译 server (Linux amd64)...")
    run_command("GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/server", cwd=PROJECT_ROOT, timeout=120)
    log_success("server 编译完成")
    
    # 编译 client (本地平台)
    log_info("编译 client (本地平台)...")
    run_command("go build -o bin/client ./cmd/client", cwd=PROJECT_ROOT, timeout=120)
    log_success("client 编译完成")

def deploy_server(config):
    """部署 server 到远程服务器"""
    log_header("步骤 3: 部署 server")
    
    ssh_config = config['server']['ssh']
    server_config = config['server']['tunnox-server']
    ssh_key = Path(__file__).parent / ssh_config['ssh-key']
    
    server_bin = PROJECT_ROOT / "bin" / "server"
    
    # 上传 server 二进制文件
    log_info("上传 server 到远程服务器...")
    scp_cmd = (
        f"scp -i {ssh_key} "
        f"-o StrictHostKeyChecking=no "
        f"-o UserKnownHostsFile=/dev/null "
        f"-o LogLevel=ERROR "
        f"{server_bin} "
        f"{ssh_config['user-name']}@{ssh_config['address']}:/opt/tunnox/tunnox-server"
    )
    run_command(scp_cmd, timeout=60)
    log_success("server 上传完成")
    
    # 设置执行权限
    log_info("设置执行权限...")
    ssh_command(config, "chmod +x /opt/tunnox/tunnox-server")
    log_success("权限设置完成")

def deploy_clients(config):
    """部署客户端到本地目录"""
    log_header("步骤 4: 部署客户端")
    
    client_bin = PROJECT_ROOT / "bin" / "client"
    
    # 部署 listen-client
    listen_folder = Path(config['listen-client']['folder']).expanduser()
    log_info(f"部署 listen-client 到 {listen_folder}...")
    listen_folder.mkdir(parents=True, exist_ok=True)
    run_command(f"cp {client_bin} {listen_folder}/client")
    log_success("listen-client 部署完成")
    
    # 部署 target-client
    target_folder = Path(config['target-client']['folder']).expanduser()
    log_info(f"部署 target-client 到 {target_folder}...")
    target_folder.mkdir(parents=True, exist_ok=True)
    run_command(f"cp {client_bin} {target_folder}/client")
    log_success("target-client 部署完成")

def generate_client_config(folder, client_id, secret_key, server_addr, server_port):
    """生成客户端配置文件"""
    config_content = f"""# Tunnox Client Configuration
client_id: {client_id}
auth_token: "{secret_key}"
anonymous: false

server:
  protocol: udp
  address: {server_addr}:{server_port}

log:
  level: info
  format: text
  output: file
"""
    
    config_file = Path(folder).expanduser() / "client-config.yaml"
    with open(config_file, 'w') as f:
        f.write(config_content)
    
    log_success(f"配置文件已生成: {config_file}")

def generate_configs(config):
    """生成所有配置文件"""
    log_header("步骤 5: 生成配置文件")
    
    server_config = config['server']['tunnox-server']
    server_addr = server_config['domain']
    server_port = server_config['port']
    
    # 生成 listen-client 配置
    log_info("生成 listen-client 配置...")
    generate_client_config(
        config['listen-client']['folder'],
        config['listen-client']['client-id'],
        config['listen-client']['secret-key'],
        server_addr,
        server_port
    )
    
    # 生成 target-client 配置
    log_info("生成 target-client 配置...")
    generate_client_config(
        config['target-client']['folder'],
        config['target-client']['client-id'],
        config['target-client']['secret-key'],
        server_addr,
        server_port
    )

def start_server(config):
    """启动远程服务器"""
    log_header("步骤 6: 启动服务器")
    
    log_info("启动 tunnox.service...")
    ssh_command(config, "systemctl start tunnox.service")
    time.sleep(3)
    
    # 检查服务状态
    log_info("检查服务状态...")
    status = ssh_command(config, "systemctl is-active tunnox.service", check=False)
    
    if status == "active":
        log_success("服务器启动成功")
    else:
        log_error(f"服务器启动失败，状态: {status}")
        # 显示日志
        logs = ssh_command(config, "journalctl -u tunnox.service -n 20 --no-pager", check=False)
        log_error(f"服务日志:\n{logs}")
        raise Exception("服务器启动失败")

def start_client(folder, name, daemon=True):
    """启动客户端"""
    client_path = Path(folder).expanduser() / "client"
    config_path = Path(folder).expanduser() / "client-config.yaml"
    log_path = Path(folder).expanduser() / "logs" / "client.log"
    error_log_path = Path(folder).expanduser() / "logs" / "client-error.log"
    
    log_info(f"启动 {name}...")
    
    # 确保 logs 目录存在
    log_path.parent.mkdir(parents=True, exist_ok=True)
    
    if daemon:
        # 后台运行，但保留 stderr 到错误日志文件
        # 同时切换到客户端目录，确保工作目录正确
        folder_path = Path(folder).expanduser()
        cmd = f"cd {folder_path} && nohup ./client -config {config_path} -daemon > /dev/null 2> {error_log_path} &"
        run_command(cmd, check=False)
    else:
        # 前台运行（用于调试）
        cmd = f"{client_path} -config {config_path} -daemon"
        run_command(cmd)
    
    time.sleep(2)
    log_success(f"{name} 启动完成")

def start_clients(config):
    """启动所有客户端"""
    log_header("步骤 7: 启动客户端")
    
    # 启动 target-client
    start_client(
        config['target-client']['folder'],
        "target-client"
    )
    
    # 等待 target-client 连接
    time.sleep(3)
    
    # 启动 listen-client
    start_client(
        config['listen-client']['folder'],
        "listen-client"
    )
    
    # 等待客户端建立连接
    log_info("等待客户端建立连接...")
    time.sleep(5)

def test_mysql_connection(config, round_num, total_rounds):
    """测试 MySQL 连接"""
    mysql_config = config['listen-client']['mysql-listen']
    
    log_info(f"测试轮次 {round_num}/{total_rounds}")
    
    try:
        # 连接 MySQL
        conn = pymysql.connect(
            host=mysql_config['address'],
            port=mysql_config['port'],
            user=mysql_config['user-name'],
            password=mysql_config['password'],
            connect_timeout=15,
            read_timeout=30,
            write_timeout=15,
            autocommit=True
        )
        
        cursor = conn.cursor()
        
        # 测试连接
        cursor.execute('SELECT 1 as test')
        result = cursor.fetchone()
        log_success(f"连接测试成功: {result}")
        
        # 执行配置的 SQL 查询
        sql = mysql_config['sql-script']
        log_info(f"执行查询: {sql}")
        
        start_time = time.time()
        cursor.execute(sql)
        results = cursor.fetchall()
        elapsed = time.time() - start_time
        
        log_success(f"查询成功! 返回 {len(results)} 行，耗时 {elapsed:.2f}s")
        
        # 估算数据大小
        if results:
            estimated_size = len(str(results[0])) * len(results)
            log_info(f"估算数据大小: ~{estimated_size / 1024 / 1024:.2f} MB")
        
        cursor.close()
        conn.close()
        
        return True
        
    except Exception as e:
        log_error(f"MySQL 测试失败: {e}")
        import traceback
        traceback.print_exc()
        return False

def run_mysql_tests(config):
    """运行 MySQL 测试（3次）"""
    log_header("步骤 8: 执行 MySQL 连接测试")
    
    total_rounds = 3
    
    for i in range(total_rounds):
        if test_mysql_connection(config, i + 1, total_rounds):
            log_success(f"✓ 第 {i + 1} 轮测试通过")
        else:
            log_error(f"✗ 第 {i + 1} 轮测试失败")
            log_error(f"测试在第 {i + 1} 轮失败，立即退出")
            show_logs(config)
            return False
        
        # 轮次之间等待更长时间，让 UDP 会话完全清理
        if i < total_rounds - 1:
            log_info("等待 5 秒，让会话完全清理...")
            time.sleep(5)
    
    log_success(f"所有 {total_rounds} 轮测试通过! ✓")
    return True

def show_logs(config):
    """显示相关日志"""
    log_header("查看日志")
    
    # 服务器日志
    log_info("服务器日志 (最后 20 行):")
    try:
        logs = ssh_command(config, "journalctl -u tunnox.service -n 20 --no-pager", check=False)
        print(logs)
    except:
        log_warning("无法获取服务器日志")
    
    # listen-client 日志
    log_info("\nlisten-client 日志 (最后 20 行):")
    listen_log = Path(config['listen-client']['folder']).expanduser() / "logs" / "client.log"
    if listen_log.exists():
        run_command(f"tail -n 20 {listen_log}", check=False)
    else:
        log_warning(f"日志文件不存在: {listen_log}")
    
    # listen-client 错误日志
    log_info("\nlisten-client 错误日志:")
    listen_error_log = Path(config['listen-client']['folder']).expanduser() / "logs" / "client-error.log"
    if listen_error_log.exists():
        run_command(f"cat {listen_error_log}", check=False)
    else:
        log_info("无错误日志")
    
    # target-client 日志
    log_info("\ntarget-client 日志 (最后 20 行):")
    target_log = Path(config['target-client']['folder']).expanduser() / "logs" / "client.log"
    if target_log.exists():
        run_command(f"tail -n 20 {target_log}", check=False)
    else:
        log_warning(f"日志文件不存在: {target_log}")
    
    # target-client 错误日志
    log_info("\ntarget-client 错误日志:")
    target_error_log = Path(config['target-client']['folder']).expanduser() / "logs" / "client-error.log"
    if target_error_log.exists():
        run_command(f"cat {target_error_log}", check=False)
    else:
        log_info("无错误日志")

def main():
    """主函数"""
    start_time = datetime.now()
    
    print(f"{Colors.BOLD}{Colors.BLUE}")
    print("╔════════════════════════════════════════════════════════════╗")
    print("║         Tunnox UDP 集成测试                                ║")
    print("╚════════════════════════════════════════════════════════════╝")
    print(f"{Colors.END}")
    
    try:
        # 加载配置
        config = load_config()
        
        # 1. 停止现有进程
        stop_processes(config)
        
        # 2. 编译二进制文件
        build_binaries()
        
        # 3. 部署 server
        deploy_server(config)
        
        # 4. 部署客户端
        deploy_clients(config)
        
        # 5. 生成配置文件
        generate_configs(config)
        
        # 6. 启动服务器
        start_server(config)
        
        # 7. 启动客户端
        start_clients(config)
        
        # 8. 执行 MySQL 测试
        test_passed = run_mysql_tests(config)
        
        # 测试结果
        elapsed = datetime.now() - start_time
        log_header("测试完成")
        log_info(f"总耗时: {elapsed.total_seconds():.2f}s")
        
        if test_passed:
            log_success("✓ 集成测试通过!")
            return 0
        else:
            log_error("✗ 集成测试失败!")
            return 1
            
    except KeyboardInterrupt:
        log_warning("\n测试被用户中断")
        return 130
    except Exception as e:
        log_error(f"测试执行失败: {e}")
        import traceback
        traceback.print_exc()
        return 1

if __name__ == "__main__":
    sys.exit(main())
