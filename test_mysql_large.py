#!/usr/bin/env python3
import sys
import signal
import pymysql
import time

def timeout_handler(signum, frame):
    raise TimeoutError("Test timed out")

signal.signal(signal.SIGALRM, timeout_handler)

def test_mysql_small():
    """Test small data packet (LIMIT 1) - should work quickly"""
    print("\n" + "=" * 60)
    print("Step 1: Testing SMALL data packet (LIMIT 1)")
    print("=" * 60)
    try:
        signal.alarm(30)  # 30 second timeout for small query
        
        conn = pymysql.connect(
            host='127.0.0.1',
            port=7788,
            user='root',
            password='dtcpay',
            database='log',
            connect_timeout=10,
            read_timeout=20,
            write_timeout=20,
            autocommit=True
        )
        cursor = conn.cursor()
        start_time = time.time()
        cursor.execute('SELECT t.* FROM log.log_db_record t LIMIT 1')
        result = cursor.fetchone()
        elapsed = time.time() - start_time
        print(f'✅ Small query SUCCESS: got 1 row in {elapsed:.2f}s')
        cursor.close()
        conn.close()
        signal.alarm(0)
        return True
    except TimeoutError as e:
        print(f"❌ Small query TIMEOUT: {e}")
        signal.alarm(0)
        return False
    except Exception as e:
        print(f"❌ Small query ERROR: {e}")
        signal.alarm(0)
        return False

def test_mysql_large():
    """Test large data packet (LIMIT 1000) - this is the real test"""
    print("\n" + "=" * 60)
    print("Step 2: Testing LARGE data packet (LIMIT 1000)")
    print("=" * 60)
    try:
        signal.alarm(120)  # 120 second timeout for large query
        
        conn = pymysql.connect(
            host='127.0.0.1',
            port=7788,
            user='root',
            password='dtcpay',
            database='log',
            connect_timeout=10,
            read_timeout=100,
            write_timeout=100,
            autocommit=True
        )
        cursor = conn.cursor()
        start_time = time.time()
        cursor.execute('SELECT t.* FROM log.log_db_record t LIMIT 1000')
        results = cursor.fetchall()
        elapsed = time.time() - start_time
        print(f'✅ Large query SUCCESS: got {len(results)} rows in {elapsed:.2f}s')
        cursor.close()
        conn.close()
        signal.alarm(0)
        return True
    except TimeoutError as e:
        print(f"❌ Large query TIMEOUT: {e}")
        signal.alarm(0)
        return False
    except Exception as e:
        print(f"❌ Large query ERROR: {e}")
        import traceback
        traceback.print_exc()
        signal.alarm(0)
        return False

if __name__ == "__main__":
    print("\n" + "=" * 60)
    print("MySQL Data Packet Transmission Test")
    print("=" * 60)
    
    # Step 1: Test small packet first to verify basic functionality
    small_ok = test_mysql_small()
    
    if not small_ok:
        print("\n" + "=" * 60)
        print("❌ Small packet test FAILED - skipping large packet test")
        print("=" * 60)
        sys.exit(1)
    
    # Step 2: Test large packet only if small packet succeeds
    time.sleep(2)  # Small delay between tests
    large_ok = test_mysql_large()
    
    print("\n" + "=" * 60)
    if small_ok and large_ok:
        print("✅ ALL TESTS PASSED")
        print("=" * 60)
        sys.exit(0)
    else:
        print("❌ SOME TESTS FAILED")
        print(f"   Small packet: {'✅' if small_ok else '❌'}")
        print(f"   Large packet: {'✅' if large_ok else '❌'}")
        print("=" * 60)
        sys.exit(1)

