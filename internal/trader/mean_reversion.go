package trader

import (
	"sync"
	"time"
)

type MeanReversionConfig struct {
	LadderLevels        int
	LevelSpacingBps     float64
	QuoteQty            float64
	RebuildInterval     time.Duration
	TradesExecutedTopic string
}

type MeanReversionStrategy struct {
	cfg          MeanReversionConfig
	mu           sync.RWMutex
	lastRebuild  time.Time
	currentLevel int
}

func NewMeanReversionStrategy(cfg MeanReversionConfig) *MeanReversionStrategy {
	return &MeanReversionStrategy{
		cfg: cfg,
	}
}

func (m *MeanReversionStrategy) Name() string {
	return "MeanReversion"
}

func (m *MeanReversionStrategy) OnPriceTick(tick PriceTick, inventory float64) *Quote {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Rebuild ladder periodically by cycling through levels
	now := time.Now()
	if now.Sub(m.lastRebuild) > m.cfg.RebuildInterval {
		m.currentLevel = (m.currentLevel + 1) % m.cfg.LadderLevels
		m.lastRebuild = now
	}

	// Place orders at specific ladder level
	// This creates constant cancel/replace activity as we cycle through levels
	levelSpacing := tick.Mid * (m.cfg.LevelSpacingBps / 10000.0)

	// Buy ladder: levels below mid
	buyLevel := m.currentLevel + 1
	bidPrice := tick.Mid - float64(buyLevel)*levelSpacing

	// Sell ladder: levels above mid
	sellLevel := m.currentLevel + 1
	askPrice := tick.Mid + float64(sellLevel)*levelSpacing

	return &Quote{
		BidPrice: bidPrice,
		BidQty:   m.cfg.QuoteQty,
		AskPrice: askPrice,
		AskQty:   m.cfg.QuoteQty,
	}
}
