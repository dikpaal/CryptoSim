package models

import (
	"time"

	"github.com/google/uuid"
)

type Order struct {
	ID        string
	MMID      string
	Symbol    string
	Side      Side
	OrderType OrderType
	Price     float64
	Qty       float64
	FilledQty float64
	Status    OrderStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewOrder(mmID, symbol string, side Side, orderType OrderType, price, qty float64) *Order {
	now := time.Now()
	return &Order{
		ID:        uuid.New().String(),
		MMID:      mmID,
		Symbol:    symbol,
		Side:      side,
		OrderType: orderType,
		Price:     price,
		Qty:       qty,
		FilledQty: 0,
		Status:    Pending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (o *Order) RemainingQty() float64 {
	return o.Qty - o.FilledQty
}

func (o *Order) IsFilled() bool {
	return o.FilledQty >= o.Qty
}

func (o *Order) Fill(qty float64) {
	o.FilledQty += qty
	o.UpdatedAt = time.Now()
	if o.IsFilled() {
		o.Status = Filled
	} else if o.FilledQty > 0 {
		o.Status = Partial
	}
}
