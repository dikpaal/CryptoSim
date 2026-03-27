package persistence

import (
	"log"

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

	return &NATSConn{
		nc: nc,
	}, nil
}

func (n *NATSConn) Close() {
	if n.nc != nil {
		n.nc.Close()
		log.Println("NATS connection closed")
	}
}
