package main

import (
	"cryptosim/internal/market-maker"
	"log"
	"os"
	"os/signal"
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
		QuoteQty:          0.015,
		MomentumThreshold: 0.0002,
		EMAWindow:         10,
	})

	mm := marketmaker.NewMarketMaker(nc, marketmaker.Config{
		ID:           mmID,
		Symbol:       symbol,
		MaxInventory: 0.5,
		MaxOrders:    8,
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
