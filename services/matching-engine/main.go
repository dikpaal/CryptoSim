package matchingengine

import (
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	symbol := getEnv("SYMBOL", "BTC-USD")

	engine := NewEngine(symbol)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/orders", engine.handleSubmitOrder)
	r.Delete("/orders/{id}", engine.handleCancelOrder)
	// r.Get("/orderbook", engine.handleGetOrderBook)
	// r.Get("/trades", engine.handleGetTrades)
	// r.Get("/orders/{id}", engine.handleGetOrder)
	// r.Get("/health", engine.handleHealth)
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
