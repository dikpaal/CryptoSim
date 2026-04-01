package trader

import (
	"math/rand"
	"sync"
	"time"
)

type NoiseConfig struct {
	MinQty              float64
	MaxQty              float64
	MinIntervalMs       int
	MaxIntervalMs       int
	MarketOrderPct      float64 // 0-100
	MaxSpreadBps        float64
	TradesExecutedTopic string
}

type NoiseStrategy struct {
	cfg          NoiseConfig
	mu           sync.RWMutex
	rng          *rand.Rand
	lastFireTime time.Time
	nextInterval time.Duration
}

func NewNoiseStrategy(cfg NoiseConfig) *NoiseStrategy {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return &NoiseStrategy{
		cfg: cfg,
		rng: rng,
	}
}

func (n *NoiseStrategy) Name() string {
	return "Noise"
}

func (n *NoiseStrategy) OnPriceTick(tick PriceTick, inventory float64) *Quote {
	n.mu.Lock()
	defer n.mu.Unlock()

	now := time.Now()

	// First tick: initialize
	if n.lastFireTime.IsZero() {
		n.lastFireTime = now
		n.nextInterval = n.randomInterval()
		return nil
	}

	// Check if enough time has passed
	if now.Sub(n.lastFireTime) < n.nextInterval {
		return nil
	}

	// Fire random order
	n.lastFireTime = now
	n.nextInterval = n.randomInterval()

	isMarket := n.rng.Float64()*100 < n.cfg.MarketOrderPct
	isBuy := n.rng.Float64() < 0.5
	qty := n.cfg.MinQty + n.rng.Float64()*(n.cfg.MaxQty-n.cfg.MinQty)

	var bidPrice, askPrice, bidQty, askQty float64

	if isBuy {
		bidQty = qty
		if isMarket {
			bidPrice = tick.Ask // market buy
		} else {
			// Random limit price within spread
			maxOffset := tick.Mid * (n.cfg.MaxSpreadBps / 10000.0)
			offset := n.rng.Float64() * maxOffset
			bidPrice = tick.Mid - offset
		}
	} else {
		askQty = qty
		if isMarket {
			askPrice = tick.Bid // market sell
		} else {
			// Random limit price within spread
			maxOffset := tick.Mid * (n.cfg.MaxSpreadBps / 10000.0)
			offset := n.rng.Float64() * maxOffset
			askPrice = tick.Mid + offset
		}
	}

	return &Quote{
		BidPrice: bidPrice,
		BidQty:   bidQty,
		AskPrice: askPrice,
		AskQty:   askQty,
	}
}

func (n *NoiseStrategy) randomInterval() time.Duration {
	ms := n.cfg.MinIntervalMs + n.rng.Intn(n.cfg.MaxIntervalMs-n.cfg.MinIntervalMs+1)
	return time.Duration(ms) * time.Millisecond
}
