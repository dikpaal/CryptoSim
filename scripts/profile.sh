#!/bin/bash

# Quick profiling helper for matching engine

ENGINE_URL=${ENGINE_URL:-http://localhost:8081}
DURATION=${DURATION:-30}
OUTPUT_DIR="profiles_$(date +%Y%m%d_%H%M%S)"

mkdir -p "$OUTPUT_DIR"

echo "=== Profiling Matching Engine ==="
echo "URL: $ENGINE_URL"
echo "Duration: ${DURATION}s"
echo "Output: $OUTPUT_DIR"
echo ""

# Check if engine is running
if ! curl -sf "$ENGINE_URL/health" > /dev/null 2>&1; then
    echo "Error: Engine not responding at $ENGINE_URL"
    exit 1
fi

echo "Collecting CPU profile..."
curl -s "$ENGINE_URL/debug/pprof/profile?seconds=$DURATION" > "$OUTPUT_DIR/cpu.pprof"
echo "✓ CPU profile saved"

echo ""
echo "Collecting heap profile..."
curl -s "$ENGINE_URL/debug/pprof/heap" > "$OUTPUT_DIR/heap.pprof"
echo "✓ Heap profile saved"

echo ""
echo "Collecting goroutine profile..."
curl -s "$ENGINE_URL/debug/pprof/goroutine" > "$OUTPUT_DIR/goroutine.pprof"
echo "✓ Goroutine profile saved"

echo ""
echo "Collecting allocs profile..."
curl -s "$ENGINE_URL/debug/pprof/allocs" > "$OUTPUT_DIR/allocs.pprof"
echo "✓ Allocs profile saved"

echo ""
echo "=== Analysis Commands ==="
echo ""
echo "CPU hotspots:"
echo "  go tool pprof -http=:8080 $OUTPUT_DIR/cpu.pprof"
echo ""
echo "Memory allocations:"
echo "  go tool pprof -http=:8080 $OUTPUT_DIR/heap.pprof"
echo ""
echo "Goroutine leaks:"
echo "  go tool pprof -http=:8080 $OUTPUT_DIR/goroutine.pprof"
echo ""
echo "Top allocations:"
echo "  go tool pprof -top $OUTPUT_DIR/allocs.pprof"
echo ""

# Auto-open CPU profile if available
if command -v go &> /dev/null; then
    echo "Opening CPU profile in browser..."
    go tool pprof -http=:8080 "$OUTPUT_DIR/cpu.pprof" &
    echo "Profile viewer at http://localhost:8080"
fi
