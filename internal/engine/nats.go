package engine

import (
	"cryptosim/internal/models"
	"encoding/json"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	TradesExecutedTopic    = "trades.executed"
	OrderBookSnapshotTopic = "orderbook.snapshot"
	OrdersSubmitTopic      = "orders.submit"
	OrdersCancelTopic      = "orders.cancel"
)

type NATSConn struct {
	nc *nats.Conn
}

func NewNATSConn(url string) (*NATSConn, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, err
	}

	return &NATSConn{nc: nc}, nil
}

func (n *NATSConn) PublishTrade(trade *models.Trade) error {
	data, err := json.Marshal(trade)
	if err != nil {
		return err
	}

	return n.nc.Publish(TradesExecutedTopic, data)
}

func (n *NATSConn) PublishOrderBookSnapshot(symbol string, bids, asks [][2]float64) error {
	snapshot := map[string]interface{}{
		"symbol": symbol,
		"bids":   bids,
		"asks":   asks,
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	return n.nc.Publish(OrderBookSnapshotTopic, data)
}

func (n *NATSConn) Close() {
	if n.nc != nil {
		n.nc.Close()
		log.Println("NATS connection closed")
	}
}

func startSnapshotPublisher(engine *Engine, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if engine.natsConn != nil {
			bids, asks := engine.orderBook.GetSnapshot(10)
			if err := engine.natsConn.PublishOrderBookSnapshot(engine.orderBook.symbol, bids, asks); err != nil {
				log.Printf("Error publishing snapshot: %v", err)
			}
		}
	}
}

// NATS request payload for orders.submit
type OrderSubmitRequest struct {
	ClientOrderID string  `json:"client_order_id"`
	MMID          string  `json:"mm_id"`
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	Type          string  `json:"type"`
	Price         float64 `json:"price"`
	Qty           float64 `json:"qty"`
	Timestamp     int64   `json:"timestamp"`
}

// NATS reply payload for orders.submit
type OrderSubmitReply struct {
	ClientOrderID string `json:"client_order_id"`
	OrderID       string `json:"order_id"`
	Accepted      bool   `json:"accepted"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
}

// NATS request payload for orders.cancel
type OrderCancelRequest struct {
	ClientCancelID string `json:"client_cancel_id"`
	MMID           string `json:"mm_id"`
	OrderID        string `json:"order_id"`
	Timestamp      int64  `json:"timestamp"`
}

// NATS reply payload for orders.cancel
type OrderCancelReply struct {
	ClientCancelID string `json:"client_cancel_id"`
	Accepted       bool   `json:"accepted"`
	Reason         string `json:"reason,omitempty"`
}

// subscribes to orders.submit and orders.cancel
func (n *NATSConn) StartRequestReplyHandlers(engine *Engine) error {
	// orders.submit
	_, err := n.nc.QueueSubscribe(OrdersSubmitTopic, "engine", func(msg *nats.Msg) {
		var req OrderSubmitRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			log.Printf("Error unmarshaling submit request: %v", err)
			reply := OrderSubmitReply{
				ClientOrderID: req.ClientOrderID,
				Accepted:      false,
				Reason:        "invalid request format",
			}
			replyData, _ := json.Marshal(reply)
			msg.Respond(replyData)
			return
		}

		side := models.Bid
		if req.Side == "ASK" {
			side = models.Ask
		}

		orderType := models.Limit
		if req.Type == "MARKET" {
			orderType = models.Market
		}

		order := models.NewOrder(req.MMID, req.Symbol, side, orderType, req.Price, req.Qty)
		trades := engine.orderBook.SubmitOrder(order)

		for _, trade := range trades {
			engine.trades = append(engine.trades, trade)
			n.PublishTrade(trade)
		}

		reply := OrderSubmitReply{
			ClientOrderID: req.ClientOrderID,
			OrderID:       order.ID,
			Accepted:      true,
			Status:        string(order.Status),
		}
		replyData, _ := json.Marshal(reply)
		msg.Respond(replyData)
	})
	if err != nil {
		return err
	}
	log.Println("Subscribed to orders.submit")

	// orders.cancel
	_, err = n.nc.QueueSubscribe(OrdersCancelTopic, "engine", func(msg *nats.Msg) {
		var req OrderCancelRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			log.Printf("Error unmarshaling cancel request: %v", err)
			reply := OrderCancelReply{
				ClientCancelID: req.ClientCancelID,
				Accepted:       false,
				Reason:         "invalid request format",
			}
			replyData, _ := json.Marshal(reply)
			msg.Respond(replyData)
			return
		}

		success := engine.orderBook.CancelOrder(req.OrderID)

		reply := OrderCancelReply{
			ClientCancelID: req.ClientCancelID,
			Accepted:       success,
		}
		if !success {
			reply.Reason = "order not found"
		}
		replyData, _ := json.Marshal(reply)
		msg.Respond(replyData)
	})
	if err != nil {
		return err
	}
	log.Println("Subscribed to orders.cancel")

	return nil
}
