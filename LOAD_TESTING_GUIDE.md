  1. cmd/loadtest/main.go - Load test tool
    - Spawns multiple traders
    - Ramps from initial → target orders/s
    - Tracks accept rate, timeouts, errors
    - Outputs JSON results
  2. scripts/bench.sh - Benchmark suite
    - Progressive tests (100 → 10k orders/s)
    - Sustained tests (long duration)
    - Automated analysis
  3. scripts/analyze_loadtest.py - Results analyzer
    - Summary tables
    - Find max throughput
    - Optimization recommendations
    - CSV export
  4. scripts/profile.sh - Profiling helper
    - Collects CPU/heap/goroutine profiles
    - Auto-opens in browser
  5. scripts/monitor.sh - Real-time monitor
    - Live orderbook
    - NATS stats
    - Container metrics
  6. LOADTEST.md - Complete guide
  7. Makefile targets added

  Usage

  # Quick test (30s)
  make loadtest-quick

  # Standard test (60s @ 2500 orders/s)
  make loadtest

  # Full benchmark suite
  make bench

  # Profile engine
  make profile

  # Real-time monitoring
  make monitor

  Next Steps

  1. Baseline test:
  docker-compose up -d
  make bench  # Select option 1
  2. Find bottleneck:
  make profile
  3. Optimize hot path from pprof output
  4. Retest to validate improvement

  What to Expect

  Results show where system breaks:
  - Accept rate drops → engine saturated
  - Timeouts spike → NATS/request handling bottleneck
  - Errors increase → application bugs

  Analyze with scripts/analyze_loadtest.py for specific recommendations.