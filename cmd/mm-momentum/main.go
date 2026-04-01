package main

import (
	marketmaker "cryptosim/internal/market-maker"
	"cryptosim/internal/models"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/nats-io/nats.go"
)

func main() {
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")
	mmID := getEnv("MM_ID", "mm-momentum")
	symbol := getEnv("SYMBOL", "BTC-USD")

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	strategy := marketmaker.NewMomentumStrategy(marketmaker.MomentumConfig{
		SpreadBps:         10.0,
		QuoteQty:          getEnvFloat("QUOTE_QTY", 0.015),
		MomentumThreshold: 0.0002,
		EMAWindow:         10,
	})

	mm := marketmaker.NewMarketMaker(nc, marketmaker.Config{
		ID:                  mmID,
		Symbol:              symbol,
		MaxInventory:        getEnvFloat("MAX_INVENTORY", 5),
		MaxOrders:           8,
		Strategy:            strategy,
		TradesExecutedTopic: models.MomentumTradeExecutedTopic,
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
