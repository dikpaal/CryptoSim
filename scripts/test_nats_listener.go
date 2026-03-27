package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
)

var (
	tradesCount    int
	snapshotsCount int
	mu             sync.Mutex
)

func main() {
	natsURL := "nats://localhost:4222"
	if len(os.Args) > 1 {
		natsURL = os.Args[1]
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	fmt.Printf("Connected to NATS at %s\n", natsURL)
	fmt.Println("Listening for messages for 10 seconds...")
	fmt.Println("---")

	// Subscribe to trades.executed
	nc.Subscribe("trades.executed", func(m *nats.Msg) {
		mu.Lock()
		tradesCount++
		mu.Unlock()

		var trade map[string]interface{}
		json.Unmarshal(m.Data, &trade)
		fmt.Printf("[TRADE] #%d: %+v\n", tradesCount, trade)
	})

	// Subscribe to orderbook.snapshot
	nc.Subscribe("orderbook.snapshot", func(m *nats.Msg) {
		mu.Lock()
		snapshotsCount++
		mu.Unlock()

		var snapshot map[string]interface{}
		json.Unmarshal(m.Data, &snapshot)
		fmt.Printf("[SNAPSHOT] #%d: symbol=%s, bids=%d, asks=%d\n",
			snapshotsCount,
			snapshot["symbol"],
			len(snapshot["bids"].([]interface{})),
			len(snapshot["asks"].([]interface{})))
	})

	// Wait for 10 seconds or SIGINT
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	timer := time.NewTimer(10 * time.Second)
	select {
	case <-timer.C:
		fmt.Println("\n---")
		fmt.Println("10 seconds elapsed")
	case <-sigChan:
		fmt.Println("\n---")
		fmt.Println("Interrupted by user")
	}

	mu.Lock()
	fmt.Printf("Total trades received: %d\n", tradesCount)
	fmt.Printf("Total snapshots received: %d\n", snapshotsCount)
	mu.Unlock()

	if tradesCount > 0 || snapshotsCount > 0 {
		fmt.Println("\n✓ NATS message flow verified!")
		os.Exit(0)
	} else {
		fmt.Println("\n✗ No messages received")
		os.Exit(1)
	}
}
