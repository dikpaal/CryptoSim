package engine

import (
	"cryptosim/models"
	"encoding/json"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	TradesExecutedTopic    = "trades.executed"
	OrderBookSnapshotTopic = "orderbook.snapshot"
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

func (n *NATSConn) PublishOrderBookSnapshot(bids, asks [][2]float64) error {
	snapshot := map[string]interface{}{
		"bids": bids,
		"asks": asks,
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
			if err := engine.natsConn.PublishOrderBookSnapshot(bids, asks); err != nil {
				log.Printf("Error publishing snapshot: %v", err)
			}
		}
	}
}
