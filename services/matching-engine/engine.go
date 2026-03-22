package matchingengine

import "cryptosim/models"

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
