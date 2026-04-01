package trader

import (
	"sync"
)

type VWAPConfig struct {
	WindowSize          int
	OffsetBps           float64
	QuoteQty            float64
	UpdateThreshold     float64 // min % change to trigger re-quote
	TradesExecutedTopic string
}

type VWAPStrategy struct {
	cfg      VWAPConfig
	mu       sync.RWMutex
	prices   []float64
	volumes  []float64
	lastVWAP float64
}

func NewVWAPStrategy(cfg VWAPConfig) *VWAPStrategy {
	return &VWAPStrategy{
		cfg:     cfg,
		prices:  make([]float64, 0, cfg.WindowSize),
		volumes: make([]float64, 0, cfg.WindowSize),
	}
}

func (v *VWAPStrategy) Name() string {
	return "VWAP"
}

func (v *VWAPStrategy) OnPriceTick(tick PriceTick, inventory float64) *Quote {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Approximate volume using spread (tighter spread = more volume)
	volume := 1.0 / (tick.Ask - tick.Bid + 0.0001)

	v.prices = append(v.prices, tick.Mid)
	v.volumes = append(v.volumes, volume)

	if len(v.prices) > v.cfg.WindowSize {
		v.prices = v.prices[1:]
		v.volumes = v.volumes[1:]
	}

	if len(v.prices) < 2 {
		return nil
	}

	vwap := v.calculateVWAP()

	// Only re-quote if VWAP changed significantly
	if v.lastVWAP > 0 {
		change := abs(vwap-v.lastVWAP) / v.lastVWAP
		if change < v.cfg.UpdateThreshold {
			return nil
		}
	}

	v.lastVWAP = vwap

	offset := vwap * (v.cfg.OffsetBps / 10000.0)

	return &Quote{
		BidPrice: vwap - offset,
		BidQty:   v.cfg.QuoteQty,
		AskPrice: vwap + offset,
		AskQty:   v.cfg.QuoteQty,
	}
}

func (v *VWAPStrategy) calculateVWAP() float64 {
	if len(v.prices) == 0 {
		return 0
	}

	var sumPV, sumV float64
	for i := 0; i < len(v.prices); i++ {
		sumPV += v.prices[i] * v.volumes[i]
		sumV += v.volumes[i]
	}

	if sumV == 0 {
		return 0
	}

	return sumPV / sumV
}
