#!/usr/bin/env python3

import json
import sys
import os
from pathlib import Path
from datetime import datetime
from typing import List, Dict

def load_results(results_dir: str) -> Dict[str, List[Dict]]:
    """Load all JSON result files from directory."""
    results = {}

    for file_path in Path(results_dir).glob("*.json"):
        test_name = file_path.stem
        try:
            with open(file_path, 'r') as f:
                data = json.load(f)
                results[test_name] = data
        except Exception as e:
            print(f"Error loading {file_path}: {e}")

    return results

def analyze_test(test_name: str, data: List[Dict]) -> Dict:
    """Analyze single test results."""
    if not data:
        return {}

    final = data[-1]

    total_submitted = final.get('TotalSubmitted', 0)
    total_acked = final.get('TotalAcked', 0)
    total_timeouts = final.get('TotalTimeouts', 0)
    total_errors = final.get('TotalErrors', 0)
    total_trades = final.get('TotalTrades', 0)

    accept_rate = (total_acked / total_submitted * 100) if total_submitted > 0 else 0
    timeout_rate = (total_timeouts / total_submitted * 100) if total_submitted > 0 else 0
    error_rate = (total_errors / total_submitted * 100) if total_submitted > 0 else 0

    # Calculate throughput (orders per second)
    if len(data) > 1:
        first_ts = datetime.fromisoformat(data[0]['Timestamp'].replace('Z', '+00:00'))
        last_ts = datetime.fromisoformat(final['Timestamp'].replace('Z', '+00:00'))
        duration = (last_ts - first_ts).total_seconds()
        actual_throughput = total_submitted / duration if duration > 0 else 0
        actual_trades_ps = total_trades / duration if duration > 0 else 0
    else:
        actual_throughput = 0
        actual_trades_ps = final.get('TradesPerSec', 0)

    return {
        'test_name': test_name,
        'target_orders_ps': final.get('OrdersPerSec', 0),
        'actual_throughput': actual_throughput,
        'actual_trades_ps': actual_trades_ps,
        'total_submitted': total_submitted,
        'total_accepted': total_acked,
        'total_trades': total_trades,
        'accept_rate': accept_rate,
        'timeout_rate': timeout_rate,
        'error_rate': error_rate,
        'orderbook_depth': final.get('OrderbookDepth', 0),
    }

def print_summary(analyses: List[Dict]):
    """Print formatted summary table."""
    print("\n" + "="*120)
    print("LOAD TEST RESULTS SUMMARY")
    print("="*120)

    print(f"\n{'Test':<20} {'Orders/s':<12} {'Trades/s':<12} {'Accept%':<10} {'Timeout%':<10} {'Error%':<10} {'Status':<10}")
    print("-"*120)

    for analysis in sorted(analyses, key=lambda x: x['target_orders_ps']):
        test_name = analysis['test_name']
        actual_orders = analysis['actual_throughput']
        actual_trades = analysis['actual_trades_ps']
        accept_rate = analysis['accept_rate']
        timeout_rate = analysis['timeout_rate']
        error_rate = analysis['error_rate']

        # Determine status
        if accept_rate >= 95 and timeout_rate < 5:
            status = "✓ PASS"
        elif accept_rate >= 90 and timeout_rate < 10:
            status = "~ MARGINAL"
        else:
            status = "✗ FAIL"

        print(f"{test_name:<20} {actual_orders:<12.1f} {actual_trades:<12.1f} {accept_rate:<10.2f} {timeout_rate:<10.2f} {error_rate:<10.2f} {status:<10}")

    print("-"*120)

def find_max_throughput(analyses: List[Dict]) -> Dict:
    """Find maximum sustainable throughput."""
    # Filter tests where accept rate >= 95% and timeout rate < 5%
    passing_tests = [
        a for a in analyses
        if a['accept_rate'] >= 95 and a['timeout_rate'] < 5
    ]

    if not passing_tests:
        return None

    return max(passing_tests, key=lambda x: x['actual_throughput'])

def print_recommendations(analyses: List[Dict]):
    """Print optimization recommendations based on results."""
    print("\n" + "="*100)
    print("RECOMMENDATIONS")
    print("="*100)

    max_throughput = find_max_throughput(analyses)

    if max_throughput:
        print(f"\n✓ Maximum sustainable throughput:")
        print(f"  Orders/s: {max_throughput['actual_throughput']:.0f}")
        print(f"  Trades/s: {max_throughput['actual_trades_ps']:.0f}")
        print(f"  Test: {max_throughput['test_name']}")
        print(f"  Accept rate: {max_throughput['accept_rate']:.2f}%")
        print(f"  Timeout rate: {max_throughput['timeout_rate']:.2f}%")
    else:
        print("\n✗ No tests passed threshold (95% accept rate, <5% timeout)")
        print("  System is bottlenecked at all tested loads")

    # Analyze failure patterns
    failures = [a for a in analyses if a['accept_rate'] < 95 or a['timeout_rate'] >= 5]
    if failures:
        print("\nFailing tests:")
        for failure in sorted(failures, key=lambda x: x['target_orders_ps']):
            print(f"  - {failure['test_name']}: {failure['accept_rate']:.1f}% accept, {failure['timeout_rate']:.1f}% timeout")

            # Provide specific recommendations
            if failure['timeout_rate'] > 10:
                print(f"    → High timeouts suggest NATS or engine request handling bottleneck")
                print(f"    → Try: increase MaxInFlight, reduce RequestTimeout, scale engine")
            elif failure['error_rate'] > 5:
                print(f"    → High errors suggest application-level issues")
                print(f"    → Check engine logs for rejections/panics")
            elif failure['accept_rate'] < 90:
                print(f"    → Low accept rate suggests matching engine saturation")
                print(f"    → Try: optimize orderbook operations, add profiling")

    print("\nOptimization suggestions:")
    print("  1. Profile matching engine: go tool pprof http://localhost:8081/debug/pprof/profile")
    print("  2. Check NATS metrics: curl http://localhost:8222/varz")
    print("  3. Monitor GC pressure: GODEBUG=gctrace=1")
    print("  4. Increase MaxInFlight in trader config")
    print("  5. Optimize orderbook mutex contention")
    print("  6. Add object pooling (sync.Pool) for orders")

def export_csv(analyses: List[Dict], output_path: str):
    """Export results to CSV."""
    import csv

    with open(output_path, 'w', newline='') as f:
        writer = csv.DictWriter(f, fieldnames=[
            'test_name', 'target_orders_ps', 'actual_throughput', 'actual_trades_ps',
            'total_submitted', 'total_accepted', 'total_trades', 'accept_rate',
            'timeout_rate', 'error_rate', 'orderbook_depth'
        ])
        writer.writeheader()
        writer.writerows(sorted(analyses, key=lambda x: x['target_orders_ps']))

    print(f"\n✓ Results exported to: {output_path}")

def main():
    if len(sys.argv) < 2:
        print("Usage: python3 analyze_loadtest.py <results_directory>")
        sys.exit(1)

    results_dir = sys.argv[1]

    if not os.path.isdir(results_dir):
        print(f"Error: {results_dir} is not a directory")
        sys.exit(1)

    print(f"Analyzing results from: {results_dir}")

    # Load all results
    results = load_results(results_dir)

    if not results:
        print("No results found in directory")
        sys.exit(1)

    # Analyze each test
    analyses = []
    for test_name, data in results.items():
        analysis = analyze_test(test_name, data)
        if analysis:
            analyses.append(analysis)

    # Print summary
    print_summary(analyses)

    # Print recommendations
    print_recommendations(analyses)

    # Export to CSV
    csv_path = os.path.join(results_dir, "summary.csv")
    export_csv(analyses, csv_path)

if __name__ == "__main__":
    main()
