#!/usr/bin/env python3
import sys
import signal
import threading
import time

def timeout_handler(signum, frame):
    raise TimeoutError("Test timed out after 10 seconds")

def test_mysql():
    """Test MySQL connection with timeout"""
    # Set up timeout
    signal.signal(signal.SIGALRM, timeout_handler)
    signal.alarm(10)  # 10 second timeout
    
    try:
        # Try mysql.connector first
        try:
            import mysql.connector
            print("Using mysql.connector...")
            conn = mysql.connector.connect(
                host='127.0.0.1',
                port=7788,
                user='root',
                password='dtcpay',
                connection_timeout=5,
                autocommit=True
            )
            cursor = conn.cursor()
            print("Executing SQL: SELECT t.* FROM core.auto_inc t LIMIT 501")
            cursor.execute('SELECT t.* FROM core.auto_inc t LIMIT 501')
            results = cursor.fetchall()
            print(f'✅ Success! Got {len(results)} rows')
            if results:
                print(f'First row sample: {results[0][:5] if len(results[0]) > 5 else results[0]}')
            cursor.close()
            conn.close()
            signal.alarm(0)  # Cancel timeout
            return True
        except ImportError:
            # Try pymysql
            import pymysql
            print("Using pymysql...")
            conn = pymysql.connect(
                host='127.0.0.1',
                port=7788,
                user='root',
                password='dtcpay',
                connect_timeout=5,
                autocommit=True
            )
            cursor = conn.cursor()
            print("Executing SQL: SELECT t.* FROM core.auto_inc t LIMIT 501")
            cursor.execute('SELECT t.* FROM core.auto_inc t LIMIT 501')
            results = cursor.fetchall()
            print(f'✅ Success! Got {len(results)} rows')
            if results:
                print(f'First row sample: {results[0][:5] if len(results[0]) > 5 else results[0]}')
            cursor.close()
            conn.close()
            signal.alarm(0)  # Cancel timeout
            return True
    except TimeoutError as e:
        print(f"❌ {e}")
        return False
    except Exception as e:
        print(f"❌ Error: {e}")
        import traceback
        traceback.print_exc()
        signal.alarm(0)  # Cancel timeout
        return False

if __name__ == "__main__":
    success = test_mysql()
    sys.exit(0 if success else 1)

