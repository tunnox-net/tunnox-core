#!/usr/bin/env python3
import sys
import signal
import pymysql
import time

def timeout_handler(signum, frame):
    raise TimeoutError("Test timed out after 10 seconds")

signal.signal(signal.SIGALRM, timeout_handler)

def test_mysql():
    print("Using pymysql...")
    try:
        signal.alarm(10)
        
        conn = pymysql.connect(
            host='127.0.0.1',
            port=7788,
            user='root',
            password='dtcpay',
            database='core',
            connect_timeout=5,
            read_timeout=5,
            write_timeout=5,
            autocommit=True
        )
        cursor = conn.cursor()
        # 先测试一个小的查询验证连接
        cursor.execute('SELECT 1 as test')
        result = cursor.fetchone()
        print(f'✅ Connection test success: {result}')
        
        # 然后测试一个限制数量的查询（避免超过max_allowed_packet）
        cursor.execute('SELECT t.* FROM core.auto_inc t LIMIT 100')
        results = cursor.fetchall()
        print(f'✅ Query success! Got {len(results)} rows')
        cursor.close()
        conn.close()
        signal.alarm(0)
        return True
    except TimeoutError as e:
        print(f"❌ Error: {e}")
        signal.alarm(0)
        return False
    except pymysql.err.OperationalError as e:
        print(f"❌ Error: {e}")
        signal.alarm(0)
        return False
    except Exception as e:
        print(f"❌ An unexpected error occurred: {e}")
        import traceback
        traceback.print_exc()
        signal.alarm(0)
        return False

if __name__ == "__main__":
    success = test_mysql()
    sys.exit(0 if success else 1)

