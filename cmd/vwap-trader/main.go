package main

import (
	"cryptosim/internal/models"
	"cryptosim/internal/participants"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")
	participantID := getEnv("PARTICIPANT_ID", "vwap-trader-1")
	symbol := getEnv("SYMBOL", string(models.BTC_USD))

	nc, err := participants.NewNATSConn(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS at %s: %v", natsURL, err)
	}
	defer nc.Close()
	log.Printf("VWAPTrader connected to NATS at %s", natsURL)

	config := participants.ParticipantConfig{
		ID:       participantID,
		Symbol:   symbol,
		NC:       nc,
		MidPrice: 0.0,
		Position: 0.0,
		Cash:     100000.0,
	}

	trader := participants.NewVWAPTrader(config)
	if err := trader.Start(); err != nil {
		log.Fatalf("Failed to start VWAP trader: %v", err)
	}
	log.Printf("VWAPTrader %s started for %s", participantID, symbol)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down VWAPTrader...")
	nc.Close()
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
