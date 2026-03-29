package main

import (
	"context"
	"cryptosim/internal/models"
	"flag"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BenchConfig struct {
	Duration      time.Duration
	BatchSize     int
	NumWorkers    int
	MaxPoolSize   int
	DBHost        string
	DBPort        string
	DBName        string
	DBUser        string
	DBPassword    string
}

type BenchResult struct {
	TotalWrites   uint64
	Duration      time.Duration
	WritesPerSec  float64
	AvgLatencyMs  float64
	P50LatencyMs  float64
	P99LatencyMs  float64
}

func main() {
	config := BenchConfig{}

	flag.DurationVar(&config.Duration, "duration", 30*time.Second, "benchmark duration")
	flag.IntVar(&config.BatchSize, "batch", 1000, "batch size for writes")
	flag.IntVar(&config.NumWorkers, "workers", 4, "number of concurrent workers")
	flag.IntVar(&config.MaxPoolSize, "pool-size", 10, "max DB pool size")
	flag.StringVar(&config.DBHost, "host", "localhost", "DB host")
	flag.StringVar(&config.DBPort, "port", "6543", "DB port")
	flag.StringVar(&config.DBName, "db", "cryptosim", "DB name")
	flag.StringVar(&config.DBUser, "user", "postgres", "DB user")
	flag.StringVar(&config.DBPassword, "password", "password", "DB password")
	flag.Parse()

	log.Printf("DB Write Benchmark")
	log.Printf("Duration: %v, BatchSize: %d, Workers: %d, PoolSize: %d",
		config.Duration, config.BatchSize, config.NumWorkers, config.MaxPoolSize)
	log.Println("---")

	result := runBenchmark(config)

	log.Println("---")
	log.Printf("RESULTS:")
	log.Printf("  Total Writes: %d trades", result.TotalWrites)
	log.Printf("  Duration: %.2fs", result.Duration.Seconds())
	log.Printf("  Throughput: %.1f writes/s", result.WritesPerSec)
	log.Printf("  Avg Latency: %.2fms", result.AvgLatencyMs)
	log.Printf("  P50 Latency: %.2fms", result.P50LatencyMs)
	log.Printf("  P99 Latency: %.2fms", result.P99LatencyMs)
}

func runBenchmark(config BenchConfig) BenchResult {
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?pool_max_conns=%d&sslmode=disable",
		config.DBUser, config.DBPassword, config.DBHost, config.DBPort, config.DBName,
		config.MaxPoolSize,
	)

	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	poolConfig.MaxConns = int32(config.MaxPoolSize)
	poolConfig.MinConns = int32(config.MaxPoolSize / 2)
	poolConfig.MaxConnLifetime = 1 * time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	db, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	log.Println("Connected to DB")

	var totalWrites uint64
	var wg sync.WaitGroup
	var latencies []time.Duration
	var latencyMu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), config.Duration)
	defer cancel()

	startTime := time.Now()

	for i := 0; i < config.NumWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			localWrites := uint64(0)
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					atomic.AddUint64(&totalWrites, localWrites)
					return
				case <-ticker.C:
					trades := generateTrades(config.BatchSize)

					writeStart := time.Now()
					err := writeTradesToDB(db, trades)
					latency := time.Since(writeStart)

					if err != nil {
						log.Printf("Worker %d write failed: %v", workerID, err)
						continue
					}

					localWrites += uint64(len(trades))

					latencyMu.Lock()
					latencies = append(latencies, latency)
					latencyMu.Unlock()
				}
			}
		}(i)
	}

	// Progress reporter
	progressTicker := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				progressTicker.Stop()
				return
			case <-progressTicker.C:
				elapsed := time.Since(startTime).Seconds()
				current := atomic.LoadUint64(&totalWrites)
				log.Printf("Progress: %d trades written (%.1f writes/s)", current, float64(current)/elapsed)
			}
		}
	}()

	wg.Wait()
	elapsed := time.Since(startTime)

	// Calculate stats
	totalWritesFinal := atomic.LoadUint64(&totalWrites)
	writesPerSec := float64(totalWritesFinal) / elapsed.Seconds()

	// Latency stats
	avgLatency, p50, p99 := calculateLatencyStats(latencies)

	return BenchResult{
		TotalWrites:  totalWritesFinal,
		Duration:     elapsed,
		WritesPerSec: writesPerSec,
		AvgLatencyMs: avgLatency,
		P50LatencyMs: p50,
		P99LatencyMs: p99,
	}
}

func generateTrades(count int) []models.Trade {
	trades := make([]models.Trade, count)
	now := time.Now()

	for i := 0; i < count; i++ {
		trades[i] = models.Trade{
			TradeID:       uuid.New().String(),
			Symbol:        "BTC-USD",
			Price:         67000.0 + float64(i%1000),
			Qty:           0.01,
			BuyerMMID:     "mm-bench-buyer",
			SellerMMID:    "mm-bench-seller",
			BuyerOrderID:  uuid.New().String(),
			SellerOrderID: uuid.New().String(),
			ExecutedAt:    now.Add(time.Duration(i) * time.Millisecond),
		}
	}

	return trades
}

func writeTradesToDB(db *pgxpool.Pool, trades []models.Trade) error {
	ctx := context.Background()

	_, err := db.CopyFrom(
		ctx,
		pgx.Identifier{"trades"},
		[]string{"trade_id", "symbol", "price", "qty", "buyer_mm_id", "seller_mm_id",
			"buyer_order_id", "seller_order_id", "executed_at"},
		pgx.CopyFromSlice(len(trades), func(i int) ([]any, error) {
			return []any{
				trades[i].TradeID,
				trades[i].Symbol,
				trades[i].Price,
				trades[i].Qty,
				trades[i].BuyerMMID,
				trades[i].SellerMMID,
				trades[i].BuyerOrderID,
				trades[i].SellerOrderID,
				trades[i].ExecutedAt,
			}, nil
		}),
	)

	return err
}

func calculateLatencyStats(latencies []time.Duration) (avg, p50, p99 float64) {
	if len(latencies) == 0 {
		return 0, 0, 0
	}

	// Sort for percentiles
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)

	// Simple bubble sort (good enough for benchmark)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Average
	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}
	avg = float64(sum.Milliseconds()) / float64(len(latencies))

	// P50
	p50Idx := len(sorted) / 2
	p50 = float64(sorted[p50Idx].Milliseconds())

	// P99
	p99Idx := int(float64(len(sorted)) * 0.99)
	if p99Idx >= len(sorted) {
		p99Idx = len(sorted) - 1
	}
	p99 = float64(sorted[p99Idx].Milliseconds())

	return
}
