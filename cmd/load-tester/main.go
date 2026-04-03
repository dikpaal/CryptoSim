package main

import (
	"cryptosim/internal/models"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type LoadTester struct {
	nc              *nats.Conn
	numWorkers      int
	ordersPerSecond int
	symbols         []string

	ordersSubmitted atomic.Uint64
	ordersAccepted  atomic.Uint64
	ordersRejected  atomic.Uint64
	tradesExecuted  atomic.Uint64
	dbWrites        atomic.Uint64

	latencies    []time.Duration
	latencyMutex sync.Mutex
}

func NewLoadTester(natsURL string, numWorkers, ordersPerSecond int) (*LoadTester, error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, err
	}

	lt := &LoadTester{
		nc:              nc,
		numWorkers:      numWorkers,
		ordersPerSecond: ordersPerSecond,
		symbols:         []string{string(models.BTC_USD), string(models.ETH_USD), string(models.XRP_USD)},
		latencies:       make([]time.Duration, 0, 10000),
	}

	return lt, nil
}

func (lt *LoadTester) Start() error {
	// Subscribe to trades to count throughput
	_, err := lt.nc.Subscribe(models.TradesExecutedTopic, func(msg *nats.Msg) {
		lt.tradesExecuted.Add(1)
	})
	if err != nil {
		return fmt.Errorf("subscribe trades: %w", err)
	}

	// Subscribe to DB metrics
	_, err = lt.nc.Subscribe(models.MetricsDBTopic, func(msg *nats.Msg) {
		var metrics models.DBMetrics
		if err := json.Unmarshal(msg.Data, &metrics); err == nil {
			lt.dbWrites.Add(metrics.TotalWrites)
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe db metrics: %w", err)
	}

	// Start workers
	ordersPerWorker := lt.ordersPerSecond / lt.numWorkers
	intervalPerOrder := time.Second / time.Duration(ordersPerWorker)

	for i := 0; i < lt.numWorkers; i++ {
		go lt.worker(i, intervalPerOrder)
	}

	// Stats reporter
	go lt.reportStats()

	return nil
}

func (lt *LoadTester) worker(id int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		lt.submitRandomOrder()
	}
}

func (lt *LoadTester) submitRandomOrder() {
	symbol := lt.symbols[rand.Intn(len(lt.symbols))]
	side := models.Bid
	if rand.Float64() > 0.5 {
		side = models.Ask
	}

	var price float64
	switch symbol {
	case string(models.BTC_USD):
		price = 50000 + rand.Float64()*100
	case string(models.ETH_USD):
		price = 3000 + rand.Float64()*50
	case string(models.XRP_USD):
		price = 0.5 + rand.Float64()*0.1
	}

	order := models.Order{
		ID:         uuid.New().String(),
		Creator_ID: fmt.Sprintf("load-tester-%d", rand.Intn(100)),
		Symbol:     symbol,
		Side:       side,
		OrderType:  models.Limit,
		Price:      price,
		Qty:        0.01 + rand.Float64()*0.09,
		Status:     models.Pending,
		CreatedAt:  time.Now(),
	}

	data, err := json.Marshal(order)
	if err != nil {
		return
	}

	start := time.Now()
	msg, err := lt.nc.Request(models.OrdersSubmitTopic, data, 2*time.Second)
	latency := time.Since(start)

	lt.ordersSubmitted.Add(1)

	if err != nil {
		lt.ordersRejected.Add(1)
		return
	}

	var ack models.OrderAck
	if err := json.Unmarshal(msg.Data, &ack); err != nil {
		lt.ordersRejected.Add(1)
		return
	}

	if ack.Status == "PENDING" || ack.Status == "PARTIAL" || ack.Status == "FILLED" {
		lt.ordersAccepted.Add(1)
		lt.latencyMutex.Lock()
		lt.latencies = append(lt.latencies, latency)
		lt.latencyMutex.Unlock()
	} else {
		lt.ordersRejected.Add(1)
	}
}

func (lt *LoadTester) reportStats() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	lastSubmitted := uint64(0)
	lastAccepted := uint64(0)
	lastTrades := uint64(0)
	lastDBWrites := uint64(0)

	for range ticker.C {
		submitted := lt.ordersSubmitted.Load()
		accepted := lt.ordersAccepted.Load()
		rejected := lt.ordersRejected.Load()
		trades := lt.tradesExecuted.Load()
		dbWrites := lt.dbWrites.Load()

		ordersPerSec := float64(submitted-lastSubmitted) / 5.0
		acceptedPerSec := float64(accepted-lastAccepted) / 5.0
		tradesPerSec := float64(trades-lastTrades) / 5.0
		dbWritesPerSec := float64(dbWrites-lastDBWrites) / 5.0

		lt.latencyMutex.Lock()
		avgLatency := time.Duration(0)
		if len(lt.latencies) > 0 {
			total := time.Duration(0)
			for _, l := range lt.latencies {
				total += l
			}
			avgLatency = total / time.Duration(len(lt.latencies))
			lt.latencies = lt.latencies[:0] // Reset for next interval
		}
		lt.latencyMutex.Unlock()

		log.Printf("Orders: %d submitted (%.0f/s), %d accepted (%.0f/s), %d rejected | Trades: %d (%.0f/s) | DB writes: %d (%.0f/s) | Avg Latency: %v",
			submitted, ordersPerSec, accepted, acceptedPerSec, rejected, trades, tradesPerSec, dbWrites, dbWritesPerSec, avgLatency)

		lastSubmitted = submitted
		lastAccepted = accepted
		lastTrades = trades
		lastDBWrites = dbWrites
	}
}

func (lt *LoadTester) Close() {
	lt.nc.Close()
}

func main() {
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")
	numWorkers, _ := strconv.Atoi(getEnv("NUM_WORKERS", "10"))
	ordersPerSecond, _ := strconv.Atoi(getEnv("ORDERS_PER_SECOND", "1000"))

	log.Printf("Starting load tester: %d workers, target %d orders/s", numWorkers, ordersPerSecond)

	lt, err := NewLoadTester(natsURL, numWorkers, ordersPerSecond)
	if err != nil {
		log.Fatalf("Failed to create load tester: %v", err)
	}
	defer lt.Close()

	if err := lt.Start(); err != nil {
		log.Fatalf("Failed to start load tester: %v", err)
	}

	log.Println("Load tester running...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down load tester...")
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
