package trader

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
)

// Main is the entry point for running the trader service.
func Main() {
	// Read config from environment variables
	cfg := Config{
		ID:                 getEnv("TRADER_ID", "trader-1"),
		Symbol:             getEnv("SYMBOL", "BTC-USD"),
		OrdersPerSec:       getEnvInt("ORDERS_PER_SEC", 100),
		MarketOrderRatio:   getEnvFloat("MARKET_ORDER_RATIO", 0.3),
		BuyRatio:           getEnvFloat("BUY_RATIO", 0.5),
		MinQty:             getEnvFloat("MIN_QTY", 0.01),
		MaxQty:             getEnvFloat("MAX_QTY", 0.1),
		AggressiveLimitBps: getEnvFloat("AGGRESSIVE_LIMIT_BPS", 5.0),
		MaxInFlight:        getEnvInt("MAX_IN_FLIGHT", 100),
		RequestTimeout:     time.Duration(getEnvInt("REQUEST_TIMEOUT_MS", 250)) * time.Millisecond,
		MatchFriendly:      getEnvBool("MATCH_FRIENDLY", true),
		PHitMM:             getEnvFloat("P_HIT_MM", 0.6),
	}

	natsURL := getEnv("NATS_URL", "nats://localhost:4222")

	// Connect to NATS
	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS at %s: %v", natsURL, err)
	}
	defer nc.Close()
	log.Printf("Connected to NATS at %s", natsURL)

	// Create trader service
	trader, err := NewTraderService(nc, cfg)
	if err != nil {
		log.Fatalf("Failed to create trader service: %v", err)
	}

	// Start trader in background
	go trader.Run()

	// Start stats reporter
	go reportStats(trader)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down trader...")
	trader.Stop()

	// Print final stats
	stats := trader.SnapshotStats()
	log.Printf("Final stats - Submitted: %d, Accepted: %d, Rejected: %d, Timeouts: %d, Errors: %d",
		stats.Submitted, stats.AckedAccepted, stats.AckedRejected, stats.Timeouts, stats.Errors)
}

// reportStats periodically logs trader stats.
func reportStats(trader *Trader) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := trader.SnapshotStats()
		log.Printf("Stats - Submitted: %d, Accepted: %d, Rejected: %d, Timeouts: %d, Errors: %d",
			stats.Submitted, stats.AckedAccepted, stats.AckedRejected, stats.Timeouts, stats.Errors)
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
