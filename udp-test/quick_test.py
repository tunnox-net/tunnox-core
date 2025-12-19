#!/usr/bin/env python3
"""
Tunnox UDP 快速测试脚本

只执行 MySQL 连接测试，不重新部署服务
"""

import sys
import time
import yaml
from pathlib import Path

try:
    import pymysql
except ImportError:
    print("❌ 缺少 pymysql 模块，请安装: pip3 install pymysql")
    sys.exit(1)

CONFIG_FILE = Path(__file__).parent / "config.yaml"

class Colors:
    GREEN = '\033[92m'
    RED = '\033[91m'
    CYAN = '\033[96m'
    END = '\033[0m'

def log_info(msg):
    print(f"{Colors.CYAN}ℹ {msg}{Colors.END}")

def log_success(msg):
    print(f"{Colors.GREEN}✓ {msg}{Colors.END}")

def log_error(msg):
    print(f"{Colors.RED}✗ {msg}{Colors.END}")

def load_config():
    with open(CONFIG_FILE, 'r', encoding='utf-8') as f:
        return yaml.safe_load(f)

def test_mysql_connection(config, round_num):
    mysql_config = config['listen-client']['mysql-listen']
    
    log_info(f"测试轮次 {round_num}/3")
    
    try:
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
        cursor.execute('SELECT 1 as test')
        result = cursor.fetchone()
        log_success(f"连接测试成功: {result}")
        
        sql = mysql_config['sql-script']
        log_info(f"执行查询: {sql[:50]}...")
        
        start_time = time.time()
        cursor.execute(sql)
        results = cursor.fetchall()
        elapsed = time.time() - start_time
        
        log_success(f"查询成功! 返回 {len(results)} 行，耗时 {elapsed:.2f}s")
        
        cursor.close()
        conn.close()
        return True
        
    except Exception as e:
        log_error(f"测试失败: {e}")
        return False

def main():
    print("\n" + "="*60)
    print("Tunnox UDP 快速测试")
    print("="*60 + "\n")
    
    config = load_config()
    
    for i in range(3):
        if test_mysql_connection(config, i + 1):
            log_success(f"第 {i + 1} 轮测试通过\n")
        else:
            log_error(f"第 {i + 1} 轮测试失败")
            log_error(f"测试在第 {i + 1} 轮失败，立即退出\n")
            print("="*60)
            log_error("测试失败!")
            return 1
        
        if i < 2:
            time.sleep(2)
    
    print("="*60)
    log_success("所有 3 轮测试通过!")
    return 0

if __name__ == "__main__":
    sys.exit(main())
