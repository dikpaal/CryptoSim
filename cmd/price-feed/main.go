package main

import (
	pricefeed "cryptosim/internal/price-feed"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")
	symbol := getEnv("SYMBOL", "BTC-USD")

	// Connect to NATS
	natsConn, err := pricefeed.NewNATSConn(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS at %s: %v", natsURL, err)
	}
	defer natsConn.Close()
	log.Printf("Connected to NATS at %s", natsURL)

	// Create and start price feed service
	pfs := pricefeed.NewPriceFeedService(natsConn, symbol)
	if err := pfs.Start(); err != nil {
		log.Fatalf("Failed to start price feed service: %v", err)
	}
	log.Println("Price feed service started")

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down price feed service...")
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
