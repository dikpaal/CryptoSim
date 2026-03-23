package pricefeed

import (
	"cryptosim/models"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	PricesLiveTopic = "prices.live"
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

func (n *NATSConn) Close() {
	if n.nc != nil {
		n.nc.Close()
		log.Println("NATS connection closed")
	}
}

func (n *NATSConn) PublishLivePrices(trade *models.Trade) error {
}

func startLivePricesPublisher(engine *Engine, interval time.Duration) {
}
