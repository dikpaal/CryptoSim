package trader

import (
	"math/rand"
	"sync"
	"time"
)

type MomentumChaserConfig struct {
	WindowSize          int
	TrendThreshold      float64
	QuoteQty            float64
	AggressiveSpread    float64 // bps
	MarketOrderChance   float64 // 0.0-1.0
	TradesExecutedTopic string
}

type MomentumChaserStrategy struct {
	cfg    MomentumChaserConfig
	mu     sync.RWMutex
	prices []float64
	rng    *rand.Rand
}

func NewMomentumChaserStrategy(cfg MomentumChaserConfig) *MomentumChaserStrategy {
	return &MomentumChaserStrategy{
		cfg:    cfg,
		prices: make([]float64, 0, cfg.WindowSize),
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (m *MomentumChaserStrategy) Name() string {
	return "MomentumChaser"
}

func (m *MomentumChaserStrategy) OnPriceTick(tick PriceTick, inventory float64) *Quote {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.prices = append(m.prices, tick.Mid)
	if len(m.prices) > m.cfg.WindowSize {
		m.prices = m.prices[1:]
	}

	if len(m.prices) < 2 {
		return nil
	}

	trend := m.calculateTrend()
	spread := tick.Mid * (m.cfg.AggressiveSpread / 10000.0)

	var bidPrice, askPrice, bidQty, askQty float64

	if trend > m.cfg.TrendThreshold {
		// Uptrend: aggressive buys, passive sells
		bidPrice = tick.Mid + spread/4 // aggressive buy just above mid
		askPrice = tick.Mid + spread*2
		bidQty = m.cfg.QuoteQty * 1.5
		askQty = m.cfg.QuoteQty * 0.5

		// Occasionally use market orders for buys
		if m.rng.Float64() < m.cfg.MarketOrderChance {
			bidPrice = tick.Ask // market buy
		}
	} else if trend < -m.cfg.TrendThreshold {
		// Downtrend: aggressive sells, passive buys
		bidPrice = tick.Mid - spread*2
		askPrice = tick.Mid - spread/4 // aggressive sell just below mid
		bidQty = m.cfg.QuoteQty * 0.5
		askQty = m.cfg.QuoteQty * 1.5

		// Occasionally use market orders for sells
		if m.rng.Float64() < m.cfg.MarketOrderChance {
			askPrice = tick.Bid // market sell
		}
	} else {
		// No clear trend: neutral positioning
		bidPrice = tick.Mid - spread/2
		askPrice = tick.Mid + spread/2
		bidQty = m.cfg.QuoteQty
		askQty = m.cfg.QuoteQty
	}

	return &Quote{
		BidPrice: bidPrice,
		BidQty:   bidQty,
		AskPrice: askPrice,
		AskQty:   askQty,
	}
}

func (m *MomentumChaserStrategy) calculateTrend() float64 {
	if len(m.prices) < 2 {
		return 0
	}

	first := m.prices[0]
	last := m.prices[len(m.prices)-1]

	return (last - first) / first
}
