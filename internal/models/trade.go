package models

import (
	"time"

	"github.com/google/uuid"
)

type Trade struct {
	TradeID       string
	Symbol        string
	Price         float64
	Qty           float64
	BuyerID       string
	SellerID      string
	BuyerOrderID  string
	SellerOrderID string
	ExecutedAt    time.Time
}

func NewTrade(symbol string, price, qty float64, buyOrder, sellOrder *Order) *Trade {
	return &Trade{
		TradeID:       uuid.New().String(),
		Symbol:        symbol,
		Price:         price,
		Qty:           qty,
		BuyerID:       buyOrder.ID,
		SellerID:      sellOrder.ID,
		BuyerOrderID:  buyOrder.ID,
		SellerOrderID: sellOrder.ID,
		ExecutedAt:    time.Now(),
	}
}
