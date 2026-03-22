// r.Post("/orders", engine.handleSubmitOrder)
// r.Delete("/orders/{id}", engine.handleCancelOrder)
// r.Get("/orderbook", engine.handleGetOrderBook)
// r.Get("/trades", engine.handleGetTrades)
// r.Get("/orders/{id}", engine.handleGetOrder)
// r.Get("/health", engine.handleHealth)

package matchingengine

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

func (engine *Engine) handleSubmitOrder(w http.ResponseWriter, r *http.Request) {

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
		side = models.Side(models.Market)
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

func (engine *Engine) handleCancelOrder(w http.ResponseWriter, r *http.Request) {

	orderID := chi.URLParam(r, "id")
	success := engine.orderBook.CancelOrder(orderID)

	if !success {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}
