package main

import (
	"cryptosim/internal/engine"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
)

func main() {
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS at %s: %v", natsURL, err)
	}
	defer nc.Close()
	log.Printf("Engine connected to NATS at %s", natsURL)

	eng := engine.NewEngine(nc)
	if err := eng.Start(); err != nil {
		log.Fatalf("Failed to start engine: %v", err)
	}
	log.Println("Engine started")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down engine...")
	nc.Close()
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
