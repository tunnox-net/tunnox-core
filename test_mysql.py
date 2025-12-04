#!/usr/bin/env python3
import sys
import signal
import pymysql
import time

def timeout_handler(signum, frame):
    raise TimeoutError("Test timed out after 90 seconds")

signal.signal(signal.SIGALRM, timeout_handler)

def test_mysql():
    print("Using pymysql...")
    signal.alarm(90)  # 增加超时时间到90秒，因为要执行3次
    
    success_count = 0
    for i in range(3):
        try:
            print(f"\n=== Test Round {i+1}/3 ===")
            conn = pymysql.connect(
                host='127.0.0.1',
                port=7788,
                user='root',
                password='dtcpay',
                database='core',
                connect_timeout=15,
                read_timeout=15,
                write_timeout=15,
                autocommit=True
            )
            cursor = conn.cursor()
            # 先测试一个小的查询验证连接
            cursor.execute('SELECT 1 as test')
            result = cursor.fetchone()
            print(f'✅ Connection test success: {result}')
            
            # 测试大数据包查询（5000行）
            print('Executing large query: SELECT * FROM log.log_db_record LIMIT 5000...')
            cursor.execute('SELECT * FROM log.log_db_record LIMIT 5000')
            results = cursor.fetchall()
            print(f'✅ Large query success! Got {len(results)} rows')
            
            # 计算数据包大小（估算）
            if results:
                estimated_size = len(str(results[0])) * len(results)  # 粗略估算
                print(f'   Estimated data size: ~{estimated_size / 1024 / 1024:.2f} MB')
            
            cursor.close()
            conn.close()
            success_count += 1
            print(f'✅ Round {i+1} completed successfully')
            
        except TimeoutError as e:
            print(f"❌ Round {i+1} Error: {e}")
            return False
        except pymysql.err.OperationalError as e:
            print(f"❌ Round {i+1} Error: {e}")
            return False
        except Exception as e:
            print(f"❌ Round {i+1} An unexpected error occurred: {e}")
            import traceback
            traceback.print_exc()
            return False
    
    signal.alarm(0)
    print(f"\n=== All Tests Completed ===")
    print(f"Success: {success_count}/3 rounds")
    return success_count == 3

if __name__ == "__main__":
    success = test_mysql()
    sys.exit(0 if success else 1)
