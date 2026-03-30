package mm

type ScalperConfig struct {
	SpreadBps              float64
	QuoteQty               float64
	InventorySkewThreshold float64
}

type ScalperStrategy struct {
	cfg ScalperConfig
}

func NewScalperStrategy(cfg ScalperConfig) *ScalperStrategy {
	return &ScalperStrategy{
		cfg: cfg,
	}
}

func (s *ScalperStrategy) Name() string {
	return "Scalper"
}

func (s *ScalperStrategy) OnPriceTick(tick PriceTick, inventory float64) *Quote {
	spread := tick.Mid * (s.cfg.SpreadBps / 10000.0)

	bidPrice := tick.Mid - spread/2
	askPrice := tick.Mid + spread/2
	bidQty := s.cfg.QuoteQty
	askQty := s.cfg.QuoteQty

	if abs(inventory) > s.cfg.InventorySkewThreshold {
		if inventory > 0 {
			askPrice = tick.Mid - spread/4
			bidPrice = tick.Mid - spread
			askQty = s.cfg.QuoteQty * 1.5
			bidQty = s.cfg.QuoteQty * 0.5
		} else {
			bidPrice = tick.Mid + spread/4
			askPrice = tick.Mid + spread
			bidQty = s.cfg.QuoteQty * 1.5
			askQty = s.cfg.QuoteQty * 0.5
		}
	}

	return &Quote{
		BidPrice: bidPrice,
		BidQty:   bidQty,
		AskPrice: askPrice,
		AskQty:   askQty,
	}
}
