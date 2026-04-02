package participants

import (
	"log"

	"github.com/nats-io/nats.go"
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

func (nats *NATSConn) Close() {
	if nats.nc != nil {
		nats.nc.Close()
		log.Println("NATS connection closed")
	}
}
