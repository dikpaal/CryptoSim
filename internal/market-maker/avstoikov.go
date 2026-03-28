package marketmaker

import (
	"math"
	"sync"
)

type AvStoikovConfig struct {
	RiskAversion     float64
	OrderArrivalRate float64
	VarianceWindow   int
	BaseQuoteQty     float64
}

type AvStoikovStrategy struct {
	cfg AvStoikovConfig
	mu  sync.RWMutex

	inventory    float64
	priceHistory []float64
}

func NewAvStoikovStrategy(cfg AvStoikovConfig) *AvStoikovStrategy {
	return &AvStoikovStrategy{
		cfg:          cfg,
		priceHistory: make([]float64, 0, cfg.VarianceWindow),
	}
}

func (a *AvStoikovStrategy) Name() string {
	return "Avellaneda-Stoikov"
}

func (a *AvStoikovStrategy) OnPriceTick(tick PriceTick, inventory float64) *Quote {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.priceHistory = append(a.priceHistory, tick.Mid)
	if len(a.priceHistory) > a.cfg.VarianceWindow {
		a.priceHistory = a.priceHistory[1:]
	}

	if len(a.priceHistory) < 10 {
		return nil
	}

	variance := a.calculateVariance()
	gamma := a.cfg.RiskAversion
	k := a.cfg.OrderArrivalRate

	reservationPrice := tick.Mid - inventory*gamma*variance

	optimalSpread := gamma*variance + (2.0/gamma)*math.Log(1+gamma/k)

	bidPrice := reservationPrice - optimalSpread/2
	askPrice := reservationPrice + optimalSpread/2

	inventoryFactor := 1.0 / (1.0 + abs(inventory))
	quoteQty := a.cfg.BaseQuoteQty * inventoryFactor

	return &Quote{
		BidPrice: bidPrice,
		BidQty:   quoteQty,
		AskPrice: askPrice,
		AskQty:   quoteQty,
	}
}

func (a *AvStoikovStrategy) calculateVariance() float64 {
	if len(a.priceHistory) < 2 {
		return 0.01
	}

	mean := 0.0
	for _, price := range a.priceHistory {
		mean += price
	}
	mean /= float64(len(a.priceHistory))

	variance := 0.0
	for _, price := range a.priceHistory {
		diff := price - mean
		variance += diff * diff
	}
	variance /= float64(len(a.priceHistory))

	if variance < 0.0001 {
		variance = 0.0001
	}

	return variance
}
