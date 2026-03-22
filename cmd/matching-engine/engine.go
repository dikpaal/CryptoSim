package matchingengine

import (
	"cryptosim/models"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Engine struct {
	orderBook *OrderBook
	trades    []*models.Trade
	natsConn  *NATSConn
}

func NewEngine(symbol string) *Engine {
	return &Engine{
		orderBook: NewOrderBook(symbol),
		trades:    make([]*models.Trade, 0, 1000),
	}
}

type SubmitOrderRequest struct {
	MMID      string  `json:"mm_id"`
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"`
	OrderType string  `json:"order_type"`
	Price     float64 `json:"price"`
	Qty       float64 `json:"qty"`
}

type SubmitOrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

type OrderBookResponse struct {
	Bids [][2]float64 `json:"bids"`
	Asks [][2]float64 `json:"asks"`
}

func (engine *Engine) HandleSubmitOrder(w http.ResponseWriter, r *http.Request) {

	var request SubmitOrderRequest

	err := json.NewDecoder(r.Body).Decode(&request)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	side := models.Bid
	if request.Side == "ASK" {
		side = models.Ask
	}

	orderType := models.Limit
	if request.OrderType == "MARKET" {
		orderType = models.Market
	}

	order := models.NewOrder(request.MMID, request.Symbol, side, orderType, request.Price, request.Qty)
	trades := engine.orderBook.SubmitOrder(order)

	for _, trade := range trades {
		engine.trades = append(engine.trades, trade)
		if engine.natsConn != nil {
			engine.natsConn.PublishTrade(trade)
		}
	}

	response := SubmitOrderResponse{
		OrderID: order.ID,
		Status:  string(order.Status),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

}

func (engine *Engine) HandleCancelOrder(w http.ResponseWriter, r *http.Request) {

	orderID := chi.URLParam(r, "id")
	success := engine.orderBook.CancelOrder(orderID)

	if !success {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}

func (engine *Engine) HandleGetOrderBook(w http.ResponseWriter, r *http.Request) {
	asks, bids := engine.orderBook.GetSnapshot(10)

	response := OrderBookResponse{
		Bids: bids,
		Asks: asks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (engine *Engine) HandleGetTrades(w http.ResponseWriter, r *http.Request) {
	start := len(engine.trades) - 100
	if start < 0 {
		start = 0
	}

	recentTrades := engine.trades[start:]

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recentTrades)
}

func (engine *Engine) HandleGetOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")

	order, exists := engine.orderBook.GetOrder(orderID)

	if !exists {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

func (engine *Engine) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
