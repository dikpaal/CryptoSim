package trader

import (
	"testing"
)

func TestMomentumChaserStrategy_Name(t *testing.T) {
	m := NewMomentumChaserStrategy(MomentumChaserConfig{WindowSize: 10})
	if m.Name() != "MomentumChaser" {
		t.Errorf("Expected name 'MomentumChaser', got '%s'", m.Name())
	}
}

func TestMomentumChaserStrategy_OnPriceTick_InsufficientHistory(t *testing.T) {
	m := NewMomentumChaserStrategy(MomentumChaserConfig{
		WindowSize:        10,
		TrendThreshold:    0.001,
		QuoteQty:          0.1,
		AggressiveSpread:  5.0,
		MarketOrderChance: 0.0,
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

func TestMomentumChaserStrategy_OnPriceTick_Uptrend(t *testing.T) {
	m := NewMomentumChaserStrategy(MomentumChaserConfig{
		WindowSize:        5,
		TrendThreshold:    0.001,
		QuoteQty:          0.1,
		AggressiveSpread:  5.0,
		MarketOrderChance: 0.0,
	})

	// Build uptrend
	for i := 0; i < 5; i++ {
		price := 50000.0 + float64(i*100)
		m.OnPriceTick(PriceTick{Mid: price}, 0.0)
	}

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50600.0,
		Ask:    50610.0,
		Mid:    50605.0,
	}

	quote := m.OnPriceTick(tick, 0.0)
	if quote == nil {
		t.Fatal("Expected quote, got nil")
	}

	// In uptrend: aggressive buys (higher bid qty), passive sells
	if quote.BidQty <= quote.AskQty {
		t.Error("Expected higher bid qty in uptrend")
	}

	// Bid should be aggressive (above mid - spread/2)
	spread := tick.Mid * (5.0 / 10000.0)
	if quote.BidPrice < tick.Mid-spread/2 {
		t.Error("Expected aggressive bid price in uptrend")
	}
}

func TestMomentumChaserStrategy_OnPriceTick_Downtrend(t *testing.T) {
	m := NewMomentumChaserStrategy(MomentumChaserConfig{
		WindowSize:        5,
		TrendThreshold:    0.001,
		QuoteQty:          0.1,
		AggressiveSpread:  5.0,
		MarketOrderChance: 0.0,
	})

	// Build downtrend
	for i := 0; i < 5; i++ {
		price := 50000.0 - float64(i*100)
		m.OnPriceTick(PriceTick{Mid: price}, 0.0)
	}

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    49400.0,
		Ask:    49410.0,
		Mid:    49405.0,
	}

	quote := m.OnPriceTick(tick, 0.0)
	if quote == nil {
		t.Fatal("Expected quote, got nil")
	}

	// In downtrend: aggressive sells (higher ask qty), passive buys
	if quote.AskQty <= quote.BidQty {
		t.Error("Expected higher ask qty in downtrend")
	}

	// Ask should be aggressive (below mid + spread/2)
	spread := tick.Mid * (5.0 / 10000.0)
	if quote.AskPrice > tick.Mid+spread/2 {
		t.Error("Expected aggressive ask price in downtrend")
	}
}

func TestMomentumChaserStrategy_OnPriceTick_Neutral(t *testing.T) {
	m := NewMomentumChaserStrategy(MomentumChaserConfig{
		WindowSize:        5,
		TrendThreshold:    0.001,
		QuoteQty:          0.1,
		AggressiveSpread:  5.0,
		MarketOrderChance: 0.0,
	})

	// Flat prices
	for i := 0; i < 5; i++ {
		m.OnPriceTick(PriceTick{Mid: 50000.0}, 0.0)
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

	// Neutral: symmetric quantities
	if quote.BidQty != quote.AskQty {
		t.Error("Expected symmetric quantities in neutral trend")
	}
}

func TestMomentumChaserStrategy_CalculateTrend(t *testing.T) {
	m := NewMomentumChaserStrategy(MomentumChaserConfig{WindowSize: 5})

	m.prices = []float64{100.0, 102.0, 104.0, 106.0, 108.0}
	trend := m.calculateTrend()

	if trend <= 0 {
		t.Error("Expected positive trend for rising prices")
	}

	expectedTrend := (108.0 - 100.0) / 100.0
	if abs(trend-expectedTrend) > 0.001 {
		t.Errorf("Expected trend ~%f, got %f", expectedTrend, trend)
	}
}
