#!/bin/bash

# DB Write Capacity Benchmark Script
# Tests various batch sizes and pool configurations to find max throughput

set -e

DURATION=30s
DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-6543}
DB_NAME=${DB_NAME:-cryptosim}
DB_USER=${DB_USER:-postgres}
DB_PASSWORD=${DB_PASSWORD:-password}

echo "=========================================="
echo "DB Write Capacity Benchmark"
echo "=========================================="
echo "Duration: $DURATION per test"
echo "DB: $DB_HOST:$DB_PORT/$DB_NAME"
echo ""

# Build benchmark tool
echo "Building benchmark tool..."
go build -o /tmp/bench-db ./cmd/bench-db
echo ""

# Test configurations
# Format: "batch_size,workers,pool_size"
CONFIGS=(
    "100,2,5"
    "500,2,5"
    "1000,2,5"
    "1000,4,10"
    "2000,4,10"
    "5000,4,10"
    "5000,8,20"
    "10000,4,10"
    "10000,8,20"
)

RESULTS_FILE="db_bench_results_$(date +%Y%m%d_%H%M%S).txt"

echo "Results will be saved to: $RESULTS_FILE"
echo "" | tee "$RESULTS_FILE"

for config in "${CONFIGS[@]}"; do
    IFS=',' read -r batch workers pool <<< "$config"

    echo "=========================================="  | tee -a "$RESULTS_FILE"
    echo "Test: batch=$batch workers=$workers pool=$pool" | tee -a "$RESULTS_FILE"
    echo "=========================================="  | tee -a "$RESULTS_FILE"

    /tmp/bench-db \
        -duration="$DURATION" \
        -batch="$batch" \
        -workers="$workers" \
        -pool-size="$pool" \
        -host="$DB_HOST" \
        -port="$DB_PORT" \
        -db="$DB_NAME" \
        -user="$DB_USER" \
        -password="$DB_PASSWORD" 2>&1 | tee -a "$RESULTS_FILE"

    echo "" | tee -a "$RESULTS_FILE"

    # Cool down between tests
    sleep 2
done

echo "=========================================="
echo "Benchmark complete! Results in $RESULTS_FILE"
echo "=========================================="

# Find best result
echo ""
echo "Best throughput:"
grep "Throughput:" "$RESULTS_FILE" | sort -t: -k2 -n -r | head -1
