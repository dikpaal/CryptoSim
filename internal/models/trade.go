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
	BuyerMMID     string
	SellerMMID    string
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
		BuyerMMID:     buyOrder.MMID,
		SellerMMID:    sellOrder.MMID,
		BuyerOrderID:  buyOrder.ID,
		SellerOrderID: sellOrder.ID,
		ExecutedAt:    time.Now(),
	}
}
