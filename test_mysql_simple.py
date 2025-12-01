#!/usr/bin/env python3
import pymysql

def test_mysql():
    try:
        print("Using pymysql...")
        conn = pymysql.connect(
            host='127.0.0.1',
            port=7788,
            user='root',
            password='tiger',
            database='test',
            connect_timeout=10,
            read_timeout=10,
            write_timeout=10,
            autocommit=True
        )
        print("✅ Connected to MySQL successfully!")
        
        # 简单查询
        with conn.cursor() as cursor:
            cursor.execute("SELECT 1")
            result = cursor.fetchone()
            print(f"✅ Query result: {result}")
        
        conn.close()
        print("✅ Test passed!")
        return True
    except Exception as e:
        print(f"❌ Error: {e}")
        import traceback
        traceback.print_exc()
        return False

if __name__ == "__main__":
    test_mysql()

