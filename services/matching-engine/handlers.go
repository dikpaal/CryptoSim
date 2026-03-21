package main

import (
	"cryptosim/models"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

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

func (e *Engine) handleSubmitOrder(w http.ResponseWriter, r *http.Request) {
	var req SubmitOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	side := models.Bid
	if req.Side == "ASK" {
		side = models.Ask
	}

	orderType := models.Limit
	if req.OrderType == "MARKET" {
		orderType = models.Market
	}

	order := models.NewOrder(req.MMID, req.Symbol, side, orderType, req.Price, req.Qty)
	trades := e.orderBook.SubmitOrder(order)

	for _, trade := range trades {
		e.trades = append(e.trades, trade)
		if e.natsConn != nil {
			e.natsConn.PublishTrade(trade)
		}
	}

	resp := SubmitOrderResponse{
		OrderID: order.ID,
		Status:  string(order.Status),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (e *Engine) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")

	success := e.orderBook.CancelOrder(orderID)

	if !success {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}

func (e *Engine) handleGetOrderBook(w http.ResponseWriter, r *http.Request) {
	bids, asks := e.orderBook.GetSnapshot(10)

	resp := OrderBookResponse{
		Bids: bids,
		Asks: asks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (e *Engine) handleGetTrades(w http.ResponseWriter, r *http.Request) {
	start := len(e.trades) - 100
	if start < 0 {
		start = 0
	}

	recentTrades := e.trades[start:]

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recentTrades)
}

func (e *Engine) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")

	order, exists := e.orderBook.GetOrder(orderID)
	if !exists {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

func (e *Engine) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
