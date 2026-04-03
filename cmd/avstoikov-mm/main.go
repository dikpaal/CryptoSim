package main

import (
	"cryptosim/internal/models"
	"cryptosim/internal/participants"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")
	participantID := getEnv("PARTICIPANT_ID", "avstoikov-mm-1")
	symbol := getEnv("SYMBOL", string(models.XRP_USD))
	numLevels, _ := strconv.Atoi(getEnv("NUM_LEVELS", "3"))

	nc, err := participants.NewNATSConn(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS at %s: %v", natsURL, err)
	}
	defer nc.Close()
	log.Printf("AvellanedaStoikovMM connected to NATS at %s", natsURL)

	config := participants.ParticipantConfig{
		ID:       participantID,
		Symbol:   symbol,
		NC:       nc,
		MidPrice: 0.0,
		Position: 0.0,
		Cash:     100000.0,
	}

	avstoikov := participants.NewAvellanedaStoikovMM(config, numLevels)
	if err := avstoikov.Start(); err != nil {
		log.Fatalf("Failed to start Avellaneda-Stoikov MM: %v", err)
	}
	log.Printf("AvellanedaStoikovMM %s started for %s", participantID, symbol)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down AvellanedaStoikovMM...")
	nc.Close()
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
