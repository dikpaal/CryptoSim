package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cryptosim/persistence"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbName := getEnv("DB_NAME", "cryptosim")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "password")

	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPassword, dbHost, dbPort, dbName)

	// Configure pool for high throughput - benchmark shows 57k writes/s possible
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	poolConfig.MaxConns = 20 // More connections for parallel writes
	poolConfig.MinConns = 10 // Keep warm connections
	poolConfig.MaxConnLifetime = 1 * time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = 1 * time.Minute

	db, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	log.Println("Connected to TimescaleDB")

	tradesHandler, err := persistence.NewTradesHandler(natsURL, db)
	if err != nil {
		log.Fatalf("Failed to create trades handler: %v", err)
	}
	tradesHandler.SubscribeTradesExecuted()
	log.Println("Subscribed to trades.executed")

	snapshotHandler, err := persistence.NewOrderbookSnapshotHandler(natsURL, db)
	if err != nil {
		log.Fatalf("Failed to create orderbook snapshot handler: %v", err)
	}
	snapshotHandler.Subscribe()
	log.Println("Subscribed to orderbook.snapshot")

	log.Println("Persistence service running...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down persistence service...")
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
