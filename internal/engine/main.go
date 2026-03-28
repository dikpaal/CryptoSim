package engine

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func Main() {
	symbol := getEnv("SYMBOL", "BTC-USD")
	port := getEnv("PORT", "8080")
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")

	engine := NewEngine(symbol)

	natsConn, err := NewNATSConn(natsURL)
	if err != nil {
		log.Printf("Warning: NATS connection failed: %v. Running without NATS.", err)
	} else {
		engine.natsConn = natsConn
		defer natsConn.Close()
		log.Println("Connected to NATS")

		if err := natsConn.StartRequestReplyHandlers(engine); err != nil {
			log.Fatalf("Failed to start NATS request-reply handlers: %v", err)
		}

		go startSnapshotPublisher(engine, 100*time.Millisecond)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/orders", engine.HandleSubmitOrder)
	r.Delete("/orders/{id}", engine.HandleCancelOrder)
	r.Get("/orderbook", engine.HandleGetOrderBook)
	r.Get("/trades", engine.HandleGetTrades)
	r.Get("/orders/{id}", engine.HandleGetOrder)
	r.Get("/health", engine.HandleHealth)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		log.Printf("Matching engine listening on port %s for symbol %s", port, symbol)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down matching engine...")
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
