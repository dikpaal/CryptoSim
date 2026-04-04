package tui

import (
	"cryptosim/internal/models"
	"sync"
)

const maxPnLHistory = 80

type ParticipantState struct {
	mu         sync.Mutex
	ID         string
	Symbol     string
	Position   float64
	CashFlow   float64 // positive = net received, negative = net spent
	MidPrice   float64
	TradeCount int
	PnLHistory []float64
}

func NewParticipantState(id, symbol string) *ParticipantState {
	return &ParticipantState{
		ID:         id,
		Symbol:     symbol,
		PnLHistory: make([]float64, 0, maxPnLHistory),
	}
}

// PnL is mark-to-market: cash delta + open position valued at current mid.
// Must be called with mu held.
func (p *ParticipantState) pnl() float64 {
	return p.CashFlow + p.Position*p.MidPrice
}

func (p *ParticipantState) OnTrade(trade models.Trade) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if trade.BuyerID == p.ID {
		p.Position += trade.Qty
		p.CashFlow -= trade.Price * trade.Qty
		p.TradeCount++
	}
	if trade.SellerID == p.ID {
		p.Position -= trade.Qty
		p.CashFlow += trade.Price * trade.Qty
		p.TradeCount++
	}

	v := p.pnl()
	if len(p.PnLHistory) >= maxPnLHistory {
		p.PnLHistory = p.PnLHistory[1:]
	}
	p.PnLHistory = append(p.PnLHistory, v)
}

func (p *ParticipantState) UpdateMid(mid float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.MidPrice = mid
}

type ParticipantSnapshot struct {
	ID         string
	Symbol     string
	PnL        float64
	Position   float64
	CashFlow   float64
	MidPrice   float64
	TradeCount int
	PnLHistory []float64
}

func (p *ParticipantState) Snapshot() ParticipantSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()

	hist := make([]float64, len(p.PnLHistory))
	copy(hist, p.PnLHistory)

	return ParticipantSnapshot{
		ID:         p.ID,
		Symbol:     p.Symbol,
		PnL:        p.pnl(),
		Position:   p.Position,
		CashFlow:   p.CashFlow,
		MidPrice:   p.MidPrice,
		TradeCount: p.TradeCount,
		PnLHistory: hist,
	}
}
