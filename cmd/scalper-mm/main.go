package main

import (
	marketmaker "cryptosim/internal/market-maker"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/nats-io/nats.go"
)

func main() {
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")
	mmID := getEnv("MM_ID", "mm-scalper")
	symbol := getEnv("SYMBOL", "BTC-USD")

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	strategy := marketmaker.NewScalperStrategy(marketmaker.ScalperConfig{
		SpreadBps:              getEnvFloat("SPREAD_BPS", 5.0),
		QuoteQty:               getEnvFloat("QUOTE_QTY", 0.01),
		InventorySkewThreshold: 0.1,
	})

	mm := marketmaker.NewMarketMaker(nc, marketmaker.Config{
		ID:           mmID,
		Symbol:       symbol,
		MaxInventory: getEnvFloat("MAX_INVENTORY", 5),
		MaxOrders:    10,
		Strategy:     strategy,
	})

	go func() {
		if err := mm.Run(); err != nil {
			log.Fatalf("MM failed: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	mm.Stop()
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return fallback
}
