#!/bin/bash

# Real-time monitoring of CryptoSim during load tests

REFRESH_INTERVAL=2

print_header() {
    clear
    echo "=================================================="
    echo "         CryptoSim Real-Time Monitor"
    echo "=================================================="
    echo ""
}

fetch_engine_stats() {
    curl -s http://localhost:8081/orderbook 2>/dev/null || echo "{}"
}

fetch_nats_stats() {
    curl -s http://localhost:8222/varz 2>/dev/null || echo "{}"
}

print_orderbook() {
    local ob=$1
    echo "=== ORDER BOOK ==="

    # Parse orderbook depth
    local bid_count=$(echo "$ob" | jq -r '.bids | length' 2>/dev/null || echo "0")
    local ask_count=$(echo "$ob" | jq -r '.asks | length' 2>/dev/null || echo "0")

    echo "Bid levels: $bid_count"
    echo "Ask levels: $ask_count"

    # Show top 3 bids and asks
    echo ""
    echo "Top Bids:"
    echo "$ob" | jq -r '.bids[:3][] | "  \(.[0]) @ \(.[1])"' 2>/dev/null || echo "  No data"

    echo ""
    echo "Top Asks:"
    echo "$ob" | jq -r '.asks[:3][] | "  \(.[0]) @ \(.[1])"' 2>/dev/null || echo "  No data"

    echo ""
}

print_nats_stats() {
    local stats=$1
    echo "=== NATS STATS ==="

    local in_msgs=$(echo "$stats" | jq -r '.in_msgs // 0' 2>/dev/null || echo "0")
    local out_msgs=$(echo "$stats" | jq -r '.out_msgs // 0' 2>/dev/null || echo "0")
    local in_bytes=$(echo "$stats" | jq -r '.in_bytes // 0' 2>/dev/null || echo "0")
    local out_bytes=$(echo "$stats" | jq -r '.out_bytes // 0' 2>/dev/null || echo "0")

    # Convert bytes to MB
    local in_mb=$(echo "scale=2; $in_bytes / 1024 / 1024" | bc 2>/dev/null || echo "0")
    local out_mb=$(echo "scale=2; $out_bytes / 1024 / 1024" | bc 2>/dev/null || echo "0")

    echo "Messages In:  $in_msgs"
    echo "Messages Out: $out_msgs"
    echo "Bytes In:     ${in_mb} MB"
    echo "Bytes Out:    ${out_mb} MB"
    echo ""
}

print_docker_stats() {
    echo "=== DOCKER CONTAINER STATS ==="

    # Get stats for key containers
    docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" \
        matching-engine trader mm-scalper mm-momentum mm-avstoikov 2>/dev/null | head -7 || echo "No containers found"

    echo ""
}

monitor_loop() {
    while true; do
        print_header

        # Fetch stats
        local ob=$(fetch_engine_stats)
        local nats=$(fetch_nats_stats)

        print_orderbook "$ob"
        print_nats_stats "$nats"
        print_docker_stats

        echo "Refreshing every ${REFRESH_INTERVAL}s... (Ctrl+C to exit)"

        sleep $REFRESH_INTERVAL
    done
}

# Check dependencies
if ! command -v jq &> /dev/null; then
    echo "Error: jq is required for JSON parsing"
    echo "Install: brew install jq (macOS) or apt-get install jq (Linux)"
    exit 1
fi

if ! command -v bc &> /dev/null; then
    echo "Warning: bc not found, byte calculations will be skipped"
fi

monitor_loop
