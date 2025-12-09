#!/usr/bin/env python3
"""测试10个并发MySQL查询（每个7000行）"""
import pymysql
import time
import threading
import sys

def query_worker(worker_id, query_size, results):
    """单个查询worker"""
    try:
        start = time.time()
        conn = pymysql.connect(
            host='127.0.0.1',
            port=7788,
            user='root',
            password='dtcpay',
            database='core',
            connect_timeout=30,
            read_timeout=120,  # 增加到120秒
            write_timeout=30
        )
        
        cursor = conn.cursor()
        print(f"[Worker {worker_id:2d}] Starting query (limit {query_size})...", flush=True)
        cursor.execute(f'SELECT * FROM log.log_db_record LIMIT {query_size}')
        rows = cursor.fetchall()
        
        elapsed = time.time() - start
        size_mb = len(rows) * 1200 / 1024 / 1024
        
        print(f"[Worker {worker_id:2d}] ✅ SUCCESS! {len(rows)} rows ({size_mb:.2f} MB) in {elapsed:.2f}s", flush=True)
        results[worker_id] = {'success': True, 'rows': len(rows), 'elapsed': elapsed, 'size_mb': size_mb}
        
        cursor.close()
        conn.close()
        
    except Exception as e:
        elapsed = time.time() - start
        print(f"[Worker {worker_id:2d}] ❌ FAILED after {elapsed:.2f}s: {str(e)[:80]}", flush=True)
        results[worker_id] = {'success': False, 'error': str(e), 'elapsed': elapsed}

def main():
    num_workers = 10
    query_size = 7000
    
    print(f"\n{'='*70}")
    print(f"Testing {num_workers} concurrent MySQL queries")
    print(f"Each query: {query_size} rows (~{query_size * 1200 / 1024 / 1024:.2f} MB)")
    print(f"Total expected: {num_workers * query_size} rows (~{num_workers * query_size * 1200 / 1024 / 1024:.2f} MB)")
    print(f"{'='*70}\n")
    
    results = {}
    threads = []
    
    start_time = time.time()
    for i in range(num_workers):
        t = threading.Thread(target=query_worker, args=(i, query_size, results))
        t.start()
        threads.append(t)
        time.sleep(0.05)  # 错开启动
    
    for t in threads:
        t.join()
    
    total_time = time.time() - start_time
    
    # 统计
    print(f"\n{'='*70}")
    print("Results Summary:")
    print(f"{'='*70}")
    
    success_count = sum(1 for r in results.values() if r.get('success'))
    total_rows = sum(r.get('rows', 0) for r in results.values() if r.get('success'))
    total_size = sum(r.get('size_mb', 0) for r in results.values() if r.get('success'))
    
    print(f"Total workers: {num_workers}")
    print(f"Success: {success_count}/{num_workers}")
    print(f"Failed: {num_workers - success_count}/{num_workers}")
    print(f"Total rows: {total_rows}")
    print(f"Total size: {total_size:.2f} MB")
    print(f"Total time: {total_time:.2f}s")
    
    if success_count > 0:
        avg_time = sum(r.get('elapsed', 0) for r in results.values() if r.get('success')) / success_count
        print(f"Average query time: {avg_time:.2f}s")
        if total_time > 0:
            throughput = total_size / total_time
            print(f"Throughput: {throughput:.2f} MB/s")
    
    print(f"{'='*70}\n")
    
    # 详细信息
    for i in sorted(results.keys()):
        r = results[i]
        if r.get('success'):
            print(f"Worker {i:2d}: {r['rows']} rows, {r['size_mb']:.2f} MB, {r['elapsed']:.2f}s")
        else:
            print(f"Worker {i:2d}: FAILED - {r.get('error', 'Unknown')[:60]}")
    
    sys.exit(0 if num_workers - success_count == 0 else 1)

if __name__ == '__main__':
    main()
