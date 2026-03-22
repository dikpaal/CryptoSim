package main

import (
	matchingengine "cryptosim/cmd/matching-engine"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	symbol := getEnv("SYMBOL", "BTC-USD")
	port := getEnv("PORT", "8080")

	engine := matchingengine.NewEngine(symbol)

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
