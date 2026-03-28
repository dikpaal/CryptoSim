package marketmaker

import (
	"testing"
)

func TestMomentumStrategy_Name(t *testing.T) {
	m := NewMomentumStrategy(MomentumConfig{EMAWindow: 10})
	if m.Name() != "Momentum" {
		t.Errorf("Expected name 'Momentum', got '%s'", m.Name())
	}
}

func TestMomentumStrategy_OnPriceTick_InsufficientHistory(t *testing.T) {
	m := NewMomentumStrategy(MomentumConfig{
		SpreadBps:         10.0,
		QuoteQty:          0.015,
		MomentumThreshold: 0.0002,
		EMAWindow:         10,
	})

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	quote := m.OnPriceTick(tick, 0.0)
	if quote != nil {
		t.Error("Expected nil quote with insufficient history")
	}
}

func TestMomentumStrategy_OnPriceTick_NeutralMomentum(t *testing.T) {
	m := NewMomentumStrategy(MomentumConfig{
		SpreadBps:         10.0,
		QuoteQty:          0.015,
		MomentumThreshold: 0.0002,
		EMAWindow:         5,
	})

	baseMid := 50000.0
	for i := 0; i < 5; i++ {
		m.OnPriceTick(PriceTick{Mid: baseMid}, 0.0)
	}

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50000.0,
	}

	quote := m.OnPriceTick(tick, 0.0)
	if quote == nil {
		t.Fatal("Expected quote, got nil")
	}

	if quote.BidQty != quote.AskQty {
		t.Error("Expected symmetric quantities with neutral momentum")
	}

	expectedSpread := 50000.0 * (10.0 / 10000.0)
	expectedBid := 50000.0 - expectedSpread/2
	expectedAsk := 50000.0 + expectedSpread/2

	if abs(quote.BidPrice-expectedBid) > 1.0 {
		t.Errorf("Expected bid ~%f, got %f", expectedBid, quote.BidPrice)
	}
	if abs(quote.AskPrice-expectedAsk) > 1.0 {
		t.Errorf("Expected ask ~%f, got %f", expectedAsk, quote.AskPrice)
	}
}

func TestMomentumStrategy_OnPriceTick_BullishMomentum(t *testing.T) {
	m := NewMomentumStrategy(MomentumConfig{
		SpreadBps:         10.0,
		QuoteQty:          0.015,
		MomentumThreshold: 0.0002,
		EMAWindow:         5,
	})

	for i := 0; i < 5; i++ {
		m.OnPriceTick(PriceTick{Mid: 50000.0}, 0.0)
	}

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50100.0,
		Ask:    50110.0,
		Mid:    50105.0,
	}

	quote := m.OnPriceTick(tick, 0.0)
	if quote == nil {
		t.Fatal("Expected quote, got nil")
	}

	if quote.AskQty <= quote.BidQty {
		t.Error("Expected larger ask qty with bullish momentum")
	}
}

func TestMomentumStrategy_OnPriceTick_BearishMomentum(t *testing.T) {
	m := NewMomentumStrategy(MomentumConfig{
		SpreadBps:         10.0,
		QuoteQty:          0.015,
		MomentumThreshold: 0.0002,
		EMAWindow:         5,
	})

	for i := 0; i < 5; i++ {
		m.OnPriceTick(PriceTick{Mid: 50000.0}, 0.0)
	}

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    49900.0,
		Ask:    49910.0,
		Mid:    49905.0,
	}

	quote := m.OnPriceTick(tick, 0.0)
	if quote == nil {
		t.Fatal("Expected quote, got nil")
	}

	if quote.BidQty <= quote.AskQty {
		t.Error("Expected larger bid qty with bearish momentum")
	}
}

func TestMomentumStrategy_CalculateEMA(t *testing.T) {
	m := NewMomentumStrategy(MomentumConfig{
		EMAWindow: 5,
	})

	prices := []float64{100.0, 102.0, 101.0, 103.0, 104.0}
	for _, price := range prices {
		m.priceHistory = append(m.priceHistory, price)
	}

	ema := m.calculateEMA()

	if ema < 100.0 || ema > 104.0 {
		t.Errorf("EMA %f outside expected range [100, 104]", ema)
	}
}
