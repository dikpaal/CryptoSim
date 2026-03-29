#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

RESULTS_DIR="loadtest_results_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$RESULTS_DIR"

echo -e "${BLUE}=== CryptoSim Load Test Benchmark ===${NC}"
echo "Results will be saved to: $RESULTS_DIR"
echo ""

# Check if services are running
check_services() {
    echo -e "${YELLOW}Checking services...${NC}"

    if ! curl -sf http://localhost:8081/health > /dev/null 2>&1; then
        echo -e "${RED}ERROR: Matching engine not responding at localhost:8081${NC}"
        echo "Run: docker-compose up -d matching-engine"
        exit 1
    fi

    if ! nc -z localhost 4222 2>/dev/null; then
        echo -e "${RED}ERROR: NATS not running at localhost:4222${NC}"
        echo "Run: docker-compose up -d nats"
        exit 1
    fi

    echo -e "${GREEN}✓ Services OK${NC}"
    echo ""
}

# Run a single load test
run_test() {
    local orders_ps=$1
    local duration=$2
    local num_traders=$3
    local test_name=$4

    echo -e "${BLUE}Running test: $test_name${NC}"
    echo "  Orders/s: $orders_ps"
    echo "  Duration: ${duration}s"
    echo "  Traders: $num_traders"
    echo ""

    go run cmd/loadtest/main.go \
        --initial-orders "$orders_ps" \
        --target-orders "$orders_ps" \
        --ramp-duration 5s \
        --test-duration "${duration}s" \
        --num-traders "$num_traders" \
        --match-friendly true \
        --report-interval 2s \
        > "$RESULTS_DIR/${test_name}.log" 2>&1
        if compgen -G "json/loadtest_results_*.json" > /dev/null; then
            mv json/loadtest_results_*.json "$RESULTS_DIR/${test_name}.json" 2>/dev/null || true
        fi

    echo -e "${GREEN}✓ Test complete${NC}"
    echo ""

    # Cool down between tests
    echo "Cooling down for 10s..."
    sleep 10
}

# Run progressive load tests
run_progressive_tests() {
    echo -e "${YELLOW}=== Progressive Load Tests ===${NC}"
    echo "Testing with increasing load to find breaking point"
    echo ""

    run_test 100 30 2 "test_100ops"
    run_test 500 30 3 "test_500ops"
    run_test 1000 30 4 "test_1000ops"
    run_test 2000 30 5 "test_2000ops"
    run_test 3000 30 6 "test_3000ops"
    run_test 5000 30 8 "test_5000ops"
    run_test 7500 30 10 "test_7500ops"
    run_test 10000 30 12 "test_10000ops"
    run_test 20000 60 12 "test_20000ops"
    run_test 45000 120 15 "test_45000ops"
}

# Run sustained load test
run_sustained_test() {
    local target_ops=$1

    echo -e "${YELLOW}=== Sustained Load Test ===${NC}"
    echo "Testing $target_ops orders/s for 5 minutes"
    echo ""

    run_test "$target_ops" 300 8 "sustained_${target_ops}ops"
}

# Analyze results
analyze_results() {
    echo -e "${YELLOW}=== Analyzing Results ===${NC}"

    if command -v python3 &> /dev/null; then
        python3 scripts/analyze_loadtest.py "$RESULTS_DIR"
    else
        echo "Python3 not found, skipping automated analysis"
        echo "View results in: $RESULTS_DIR/*.json"
    fi
}

# Main execution
main() {
    check_services

    echo -e "${BLUE}Select test mode:${NC}"
    echo "1) Progressive tests (100 → 45000 orders/s)"
    echo "2) Sustained test at target rate"
    echo "3) Quick test (single 30s run)"
    echo "4) All tests"
    echo ""
    read -p "Enter choice [1-4]: " choice

    case $choice in
        1)
            run_progressive_tests
            ;;
        2)
            read -p "Target orders/s: " target
            run_sustained_test "$target"
            ;;
        3)
            read -p "Orders/s: " rate
            run_test "$rate" 30 5 "quick_test"
            ;;
        4)
            run_progressive_tests
            echo ""
            echo -e "${BLUE}Running sustained test at 5000 orders/s...${NC}"
            run_sustained_test 5000
            ;;
        *)
            echo -e "${RED}Invalid choice${NC}"
            exit 1
            ;;
    esac

    echo ""
    echo -e "${GREEN}=== Benchmark Complete ===${NC}"
    echo "Results saved to: $RESULTS_DIR"
    echo ""

    # Quick summary
    echo -e "${BLUE}Quick Summary:${NC}"
    for logfile in "$RESULTS_DIR"/*.log; do
        if [ -f "$logfile" ]; then
            testname=$(basename "$logfile" .log)
            echo -e "\n${YELLOW}$testname:${NC}"
            grep -E "(Total Orders|Accept Rate|Timeout)" "$logfile" | tail -5 || echo "  No summary found"
        fi
    done

    echo ""
    analyze_results
}

main "$@"
