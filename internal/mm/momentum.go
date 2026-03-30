package mm

import (
	"sync"
)

type MomentumConfig struct {
	SpreadBps         float64
	QuoteQty          float64
	MomentumThreshold float64
	EMAWindow         int
}

type MomentumStrategy struct {
	cfg MomentumConfig
	mu  sync.RWMutex

	priceHistory []float64
	ema          float64
	lastMid      float64
	momentum     float64
}

func NewMomentumStrategy(cfg MomentumConfig) *MomentumStrategy {
	return &MomentumStrategy{
		cfg:          cfg,
		priceHistory: make([]float64, 0, cfg.EMAWindow),
	}
}

func (m *MomentumStrategy) Name() string {
	return "Momentum"
}

func (m *MomentumStrategy) OnPriceTick(tick PriceTick, inventory float64) *Quote {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.priceHistory = append(m.priceHistory, tick.Mid)
	if len(m.priceHistory) > m.cfg.EMAWindow {
		m.priceHistory = m.priceHistory[1:]
	}

	if len(m.priceHistory) < 2 {
		return nil
	}

	m.ema = m.calculateEMA()
	if m.lastMid > 0 {
		m.momentum = (tick.Mid - m.ema) / m.ema
	}
	m.lastMid = tick.Mid

	spread := tick.Mid * (m.cfg.SpreadBps / 10000.0)
	bidQty := m.cfg.QuoteQty
	askQty := m.cfg.QuoteQty

	var bidPrice, askPrice float64

	if m.momentum > m.cfg.MomentumThreshold {
		bidPrice = tick.Mid - spread/4
		askPrice = tick.Mid + spread/2
		askQty = m.cfg.QuoteQty * 1.5
		bidQty = m.cfg.QuoteQty * 0.8
	} else if m.momentum < -m.cfg.MomentumThreshold {
		bidPrice = tick.Mid - spread/2
		askPrice = tick.Mid + spread/4
		bidQty = m.cfg.QuoteQty * 1.5
		askQty = m.cfg.QuoteQty * 0.8
	} else {
		bidPrice = tick.Mid - spread/2
		askPrice = tick.Mid + spread/2
	}

	return &Quote{
		BidPrice: bidPrice,
		BidQty:   bidQty,
		AskPrice: askPrice,
		AskQty:   askQty,
	}
}

func (m *MomentumStrategy) calculateEMA() float64 {
	if len(m.priceHistory) == 0 {
		return 0
	}

	alpha := 2.0 / float64(len(m.priceHistory)+1)
	ema := m.priceHistory[0]

	for i := 1; i < len(m.priceHistory); i++ {
		ema = alpha*m.priceHistory[i] + (1-alpha)*ema
	}

	return ema
}
