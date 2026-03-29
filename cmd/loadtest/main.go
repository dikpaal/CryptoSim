package main

import (
	"context"
	"cryptosim/internal/trader"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
)

type LoadTestConfig struct {
	NATSUrl          string
	Symbol           string
	InitialOrdersPS  int
	TargetOrdersPS   int
	RampUpDuration   time.Duration
	TestDuration     time.Duration
	NumTraders       int
	MarketOrderRatio float64
	BuyRatio         float64
	MinQty           float64
	MaxQty           float64
	MaxInFlight      int
	RequestTimeout   time.Duration
	MatchFriendly    bool
	ReportInterval   time.Duration
	EngineURL        string
}

type LoadTestResult struct {
	Timestamp      time.Time
	OrdersPerSec   float64
	TotalSubmitted uint64
	TotalAcked     uint64
	TotalRejected  uint64
	TotalTimeouts  uint64
	TotalErrors    uint64
	TotalTrades    uint64
	AcceptRate     float64
	TimeoutRate    float64
	TradesPerSec   float64
	EngineMetrics  *EngineMetrics
	OrderbookDepth int
	MMStatus       map[string]*MMStatus
}

type MMStatus struct {
	MMID          string  `json:"mm_id"`
	Strategy      string  `json:"strategy"`
	Inventory     float64 `json:"inventory"`
	RealizedPnL   float64 `json:"realized_pnl"`
	UnrealizedPnL float64 `json:"unrealized_pnl"`
	OpenOrders    int     `json:"open_orders"`
}

type EngineMetrics struct {
	ActiveOrders       int     `json:"active_orders"`
	TotalOrdersHandled uint64  `json:"total_orders_handled"`
	TotalTradesExec    uint64  `json:"total_trades_executed"`
	P50LatencyMicros   float64 `json:"p50_latency_micros"`
	P99LatencyMicros   float64 `json:"p99_latency_micros"`
}

type OrderbookResponse struct {
	Symbol string       `json:"symbol"`
	Bids   [][2]float64 `json:"bids"`
	Asks   [][2]float64 `json:"asks"`
}

func main() {
	cfg := parseFlags()

	log.Printf("Load Test Configuration:")
	log.Printf("  NATS URL: %s", cfg.NATSUrl)
	log.Printf("  Symbol: %s", cfg.Symbol)
	log.Printf("  Initial Orders/s: %d", cfg.InitialOrdersPS)
	log.Printf("  Target Orders/s: %d", cfg.TargetOrdersPS)
	log.Printf("  Ramp-up Duration: %s", cfg.RampUpDuration)
	log.Printf("  Test Duration: %s", cfg.TestDuration)
	log.Printf("  Num Traders: %d", cfg.NumTraders)
	log.Printf("  Market Order Ratio: %.2f", cfg.MarketOrderRatio)
	log.Printf("  Match Friendly: %v", cfg.MatchFriendly)

	nc, err := nats.Connect(cfg.NATSUrl)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()
	log.Println("Connected to NATS")

	// Create output file for results
	resultsFile, err := os.Create(fmt.Sprintf("json/loadtest_results_%s.json", time.Now().Format("20060102_150405")))
	if err != nil {
		log.Fatalf("Failed to create results file: %v", err)
	}
	defer resultsFile.Close()

	// Start load test
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("\nShutting down load test...")
		cancel()
	}()

	results := runLoadTest(ctx, nc, cfg)

	// Write results to file
	encoder := json.NewEncoder(resultsFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		log.Printf("Failed to write results: %v", err)
	}

	// Print summary
	printSummary(results)
}

func parseFlags() LoadTestConfig {
	cfg := LoadTestConfig{}

	flag.StringVar(&cfg.NATSUrl, "nats-url", "nats://localhost:4222", "NATS server URL")
	flag.StringVar(&cfg.Symbol, "symbol", "BTC-USD", "Trading symbol")
	flag.IntVar(&cfg.InitialOrdersPS, "initial-orders", 100, "Initial orders per second")
	flag.IntVar(&cfg.TargetOrdersPS, "target-orders", 5000, "Target orders per second")
	flag.DurationVar(&cfg.RampUpDuration, "ramp-duration", 60*time.Second, "Ramp-up duration")
	flag.DurationVar(&cfg.TestDuration, "test-duration", 120*time.Second, "Total test duration")
	flag.IntVar(&cfg.NumTraders, "num-traders", 5, "Number of trader instances")
	flag.Float64Var(&cfg.MarketOrderRatio, "market-ratio", 0.3, "Market order ratio")
	flag.Float64Var(&cfg.BuyRatio, "buy-ratio", 0.5, "Buy ratio")
	flag.Float64Var(&cfg.MinQty, "min-qty", 0.01, "Minimum order quantity")
	flag.Float64Var(&cfg.MaxQty, "max-qty", 0.1, "Maximum order quantity")
	flag.IntVar(&cfg.MaxInFlight, "max-in-flight", 200, "Max in-flight requests per trader")
	flag.DurationVar(&cfg.RequestTimeout, "timeout", 500*time.Millisecond, "Request timeout")
	flag.BoolVar(&cfg.MatchFriendly, "match-friendly", true, "Enable match-friendly mode")
	flag.DurationVar(&cfg.ReportInterval, "report-interval", 5*time.Second, "Metrics report interval")
	flag.StringVar(&cfg.EngineURL, "engine-url", "http://localhost:8081", "Matching engine URL")

	flag.Parse()
	return cfg
}

func runLoadTest(ctx context.Context, nc *nats.Conn, cfg LoadTestConfig) []LoadTestResult {
	var results []LoadTestResult
	var resultsMu sync.Mutex

	// Create traders
	traders := make([]*trader.Trader, cfg.NumTraders)
	var wg sync.WaitGroup

	// Track trades executed
	var tradeCount uint64
	tradeSub, err := nc.Subscribe("trades.executed", func(msg *nats.Msg) {
		tradeCount++
	})
	if err != nil {
		log.Printf("Warning: Failed to subscribe to trades.executed: %v", err)
	} else {
		defer tradeSub.Unsubscribe()
		log.Println("Subscribed to trades.executed for tracking")
	}

	// Track MM status
	mmStatus := make(map[string]*MMStatus)
	var mmMu sync.Mutex
	mmSub, err := nc.Subscribe("mm.status", func(msg *nats.Msg) {
		var status MMStatus
		if err := json.Unmarshal(msg.Data, &status); err != nil {
			return
		}
		mmMu.Lock()
		mmStatus[status.MMID] = &status
		mmMu.Unlock()
	})
	if err != nil {
		log.Printf("Warning: Failed to subscribe to mm.status: %v", err)
	} else {
		defer mmSub.Unsubscribe()
		log.Println("Subscribed to mm.status for P&L tracking")
	}

	startTime := time.Now()
	ticker := time.NewTicker(cfg.ReportInterval)
	defer ticker.Stop()

	// Calculate orders per second per trader over time
	calcOrdersPerTrader := func(elapsed time.Duration) int {
		if elapsed >= cfg.RampUpDuration {
			return cfg.TargetOrdersPS / cfg.NumTraders
		}
		progress := float64(elapsed) / float64(cfg.RampUpDuration)
		ordersPS := float64(cfg.InitialOrdersPS) + progress*float64(cfg.TargetOrdersPS-cfg.InitialOrdersPS)
		return int(ordersPS) / cfg.NumTraders
	}

	// Dynamic trader management
	var tradersMu sync.Mutex
	updateTraders := func() {
		tradersMu.Lock()
		defer tradersMu.Unlock()

		elapsed := time.Since(startTime)
		ordersPerTrader := calcOrdersPerTrader(elapsed)

		for i := 0; i < cfg.NumTraders; i++ {
			// Stop old trader if exists
			if traders[i] != nil {
				traders[i].Stop()
			}

			// Create new trader with updated rate
			traderCfg := trader.Config{
				ID:                 fmt.Sprintf("loadtest-trader-%d", i),
				Symbol:             cfg.Symbol,
				OrdersPerSec:       ordersPerTrader,
				MarketOrderRatio:   cfg.MarketOrderRatio,
				BuyRatio:           cfg.BuyRatio,
				MinQty:             cfg.MinQty,
				MaxQty:             cfg.MaxQty,
				AggressiveLimitBps: 5.0,
				MaxInFlight:        cfg.MaxInFlight,
				RequestTimeout:     cfg.RequestTimeout,
				MatchFriendly:      cfg.MatchFriendly,
				PHitMM:             0.6,
			}

			t, err := trader.NewTraderService(nc, traderCfg)
			if err != nil {
				log.Printf("Failed to create trader %d: %v", i, err)
				continue
			}

			traders[i] = t
			wg.Add(1)
			go func(tr *trader.Trader) {
				defer wg.Done()
				tr.Run()
			}(t)
		}

		log.Printf("Updated traders: %d orders/s per trader (%d total)", ordersPerTrader, ordersPerTrader*cfg.NumTraders)
	}

	// Initial trader creation
	updateTraders()

	// Periodic metrics collection
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mmMu.Lock()
				mmStatusCopy := copyMMStatus(mmStatus)
				mmMu.Unlock()

				result := collectMetrics(traders, cfg, tradeCount, startTime, mmStatusCopy)
				resultsMu.Lock()
				results = append(results, result)
				resultsMu.Unlock()

				printProgress(result)

				// Update traders during ramp-up
				elapsed := time.Since(startTime)
				if elapsed < cfg.RampUpDuration {
					updateTraders()
				}
			}
		}
	}()

	// Run test for specified duration
	testCtx, testCancel := context.WithTimeout(ctx, cfg.TestDuration)
	defer testCancel()

	<-testCtx.Done()

	// Stop all traders
	log.Println("Stopping traders...")
	tradersMu.Lock()
	for _, t := range traders {
		if t != nil {
			t.Stop()
		}
	}
	tradersMu.Unlock()

	// Wait for traders to finish
	wg.Wait()

	// Collect final metrics
	mmMu.Lock()
	mmStatusCopy := copyMMStatus(mmStatus)
	mmMu.Unlock()

	finalResult := collectMetrics(traders, cfg, tradeCount, startTime, mmStatusCopy)
	resultsMu.Lock()
	results = append(results, finalResult)
	resultsMu.Unlock()

	return results
}

func copyMMStatus(src map[string]*MMStatus) map[string]*MMStatus {
	dst := make(map[string]*MMStatus)
	for k, v := range src {
		copied := *v
		dst[k] = &copied
	}
	return dst
}

func collectMetrics(traders []*trader.Trader, cfg LoadTestConfig, tradeCount uint64, startTime time.Time, mmStatus map[string]*MMStatus) LoadTestResult {
	var totalSubmitted, totalAcked, totalRejected, totalTimeouts, totalErrors uint64

	for _, t := range traders {
		if t == nil {
			continue
		}
		stats := t.SnapshotStats()
		totalSubmitted += stats.Submitted
		totalAcked += stats.AckedAccepted
		totalRejected += stats.AckedRejected
		totalTimeouts += stats.Timeouts
		totalErrors += stats.Errors
	}

	acceptRate := 0.0
	if totalSubmitted > 0 {
		acceptRate = float64(totalAcked) / float64(totalSubmitted)
	}

	timeoutRate := 0.0
	if totalSubmitted > 0 {
		timeoutRate = float64(totalTimeouts) / float64(totalSubmitted)
	}

	// Fetch engine metrics (if available)
	engineMetrics := fetchEngineMetrics(cfg.EngineURL)
	orderbookDepth := fetchOrderbookDepth(cfg.EngineURL)

	// Calculate orders per second and trades per second
	elapsed := time.Since(startTime).Seconds()
	ordersPerSec := 0.0
	tradesPerSec := 0.0
	if elapsed > 0 {
		ordersPerSec = float64(totalAcked) / elapsed
		tradesPerSec = float64(tradeCount) / elapsed
	}

	return LoadTestResult{
		Timestamp:      time.Now(),
		OrdersPerSec:   ordersPerSec,
		TotalSubmitted: totalSubmitted,
		TotalAcked:     totalAcked,
		TotalRejected:  totalRejected,
		TotalTimeouts:  totalTimeouts,
		TotalErrors:    totalErrors,
		TotalTrades:    tradeCount,
		AcceptRate:     acceptRate,
		TimeoutRate:    timeoutRate,
		TradesPerSec:   tradesPerSec,
		EngineMetrics:  engineMetrics,
		OrderbookDepth: orderbookDepth,
		MMStatus:       mmStatus,
	}
}

func fetchEngineMetrics(engineURL string) *EngineMetrics {
	// Try to fetch metrics from engine /metrics endpoint
	// This is a placeholder - adjust based on your actual metrics endpoint
	return nil
}

func fetchOrderbookDepth(engineURL string) int {
	resp, err := http.Get(fmt.Sprintf("%s/orderbook", engineURL))
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	var ob OrderbookResponse
	if err := json.NewDecoder(resp.Body).Decode(&ob); err != nil {
		return 0
	}

	depth := len(ob.Bids) + len(ob.Asks)
	return depth
}

func printProgress(result LoadTestResult) {
	log.Printf("=== Load Test Progress ===")
	log.Printf("Time: %s", result.Timestamp.Format("15:04:05"))
	log.Printf("Orders Accepted: %d (%.1f orders/s)", result.TotalAcked, result.OrdersPerSec)
	log.Printf("Orders Submitted: %d (%.2f%% accept rate)", result.TotalSubmitted, result.AcceptRate*100)
	log.Printf("Orders Rejected: %d", result.TotalRejected)
	log.Printf("Timeouts: %d (%.2f%%)", result.TotalTimeouts, result.TimeoutRate*100)
	log.Printf("Errors: %d", result.TotalErrors)
	log.Printf("Trades Executed: %d (%.1f trades/s)", result.TotalTrades, result.TradesPerSec)
	log.Printf("Orderbook Depth: %d levels", result.OrderbookDepth)

	// Print MM P&L
	if len(result.MMStatus) > 0 {
		log.Printf("Market Makers:")
		for mmID, status := range result.MMStatus {
			totalPnL := status.RealizedPnL + status.UnrealizedPnL
			log.Printf("  %s (%s): P&L=$%.2f (R=$%.2f U=$%.2f) Inv=%.4f Orders=%d",
				mmID, status.Strategy, totalPnL, status.RealizedPnL, status.UnrealizedPnL,
				status.Inventory, status.OpenOrders)
		}
	}

	if result.EngineMetrics != nil {
		log.Printf("Engine - Active Orders: %d, Total Trades: %d",
			result.EngineMetrics.ActiveOrders, result.EngineMetrics.TotalTradesExec)
		log.Printf("Engine - P50 Latency: %.2fµs, P99: %.2fµs",
			result.EngineMetrics.P50LatencyMicros, result.EngineMetrics.P99LatencyMicros)
	}
	log.Println()
}

func printSummary(results []LoadTestResult) {
	if len(results) == 0 {
		log.Println("No results to summarize")
		return
	}

	final := results[len(results)-1]

	log.Println("\n=== Load Test Summary ===")
	log.Printf("Total Duration: %s", final.Timestamp.Sub(results[0].Timestamp))
	log.Printf("Total Orders Submitted: %d", final.TotalSubmitted)
	log.Printf("Total Orders Accepted: %d (%.1f orders/s, %.2f%% accept)", final.TotalAcked, final.OrdersPerSec, final.AcceptRate*100)
	log.Printf("Total Orders Rejected: %d", final.TotalRejected)
	log.Printf("Total Timeouts: %d (%.2f%%)", final.TotalTimeouts, final.TimeoutRate*100)
	log.Printf("Total Errors: %d", final.TotalErrors)
	log.Printf("Total Trades Executed: %d (%.1f trades/s)", final.TotalTrades, final.TradesPerSec)

	// Print final MM P&L
	if len(final.MMStatus) > 0 {
		log.Printf("\nMarket Maker Performance:")
		totalMMPnL := 0.0
		for mmID, status := range final.MMStatus {
			totalPnL := status.RealizedPnL + status.UnrealizedPnL
			totalMMPnL += totalPnL
			log.Printf("  %s (%s): Total P&L=$%.2f (Realized=$%.2f, Unrealized=$%.2f)",
				mmID, status.Strategy, totalPnL, status.RealizedPnL, status.UnrealizedPnL)
		}
		log.Printf("  Combined MM P&L: $%.2f", totalMMPnL)
		if totalMMPnL > 0 {
			log.Printf("  ✓ MMs are profitable (capturing spread from traders)")
		} else if totalMMPnL < 0 {
			log.Printf("  ⚠ MMs are losing money (adverse selection or not enough liquidity)")
		}
	}

	if final.EngineMetrics != nil {
		log.Printf("\nEngine Performance:")
		log.Printf("  Active Orders: %d", final.EngineMetrics.ActiveOrders)
		log.Printf("  Total Trades Executed: %d", final.EngineMetrics.TotalTradesExec)
		log.Printf("  P50 Matching Latency: %.2fµs", final.EngineMetrics.P50LatencyMicros)
		log.Printf("  P99 Matching Latency: %.2fµs", final.EngineMetrics.P99LatencyMicros)
	}

	// Calculate average accept rate across test
	avgAcceptRate := 0.0
	for _, r := range results {
		avgAcceptRate += r.AcceptRate
	}
	avgAcceptRate /= float64(len(results))
	log.Printf("\nAverage Accept Rate: %.2f%%", avgAcceptRate*100)
}
