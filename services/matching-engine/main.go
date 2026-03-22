package matchingengine

import (
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

	engine := NewEngine(symbol)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/orders", engine.handleSubmitOrder)
	r.Delete("/orders/{id}", engine.handleCancelOrder)
	r.Get("/orderbook", engine.handleGetOrderBook)
	r.Get("/trades", engine.handleGetTrades)
	r.Get("/orders/{id}", engine.handleGetOrder)
	r.Get("/health", engine.handleHealth)

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
