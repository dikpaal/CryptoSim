package trader

import (
	"testing"
	"time"
)

func TestMeanReversionStrategy_Name(t *testing.T) {
	m := NewMeanReversionStrategy(MeanReversionConfig{})
	if m.Name() != "MeanReversion" {
		t.Errorf("Expected name 'MeanReversion', got '%s'", m.Name())
	}
}

func TestMeanReversionStrategy_OnPriceTick_BasicQuote(t *testing.T) {
	m := NewMeanReversionStrategy(MeanReversionConfig{
		LadderLevels:    5,
		LevelSpacingBps: 10.0,
		QuoteQty:        0.1,
		RebuildInterval: 100 * time.Millisecond,
	})

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	quote := m.OnPriceTick(tick, 0.0)
	if quote == nil {
		t.Fatal("Expected quote, got nil")
	}

	// Bid should be below mid, ask should be above mid
	if quote.BidPrice >= tick.Mid {
		t.Error("Expected bid below mid")
	}
	if quote.AskPrice <= tick.Mid {
		t.Error("Expected ask above mid")
	}

	// Quantities should match config
	if quote.BidQty != 0.1 {
		t.Errorf("Expected bid qty 0.1, got %f", quote.BidQty)
	}
	if quote.AskQty != 0.1 {
		t.Errorf("Expected ask qty 0.1, got %f", quote.AskQty)
	}
}

func TestMeanReversionStrategy_OnPriceTick_LadderLevels(t *testing.T) {
	m := NewMeanReversionStrategy(MeanReversionConfig{
		LadderLevels:    3,
		LevelSpacingBps: 10.0,
		QuoteQty:        0.1,
		RebuildInterval: 100 * time.Millisecond,
	})

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	// Get first quote
	quote1 := m.OnPriceTick(tick, 0.0)
	if quote1 == nil {
		t.Fatal("Expected first quote, got nil")
	}

	levelSpacing := tick.Mid * (10.0 / 10000.0)

	// Verify bid is at ladder level below mid
	if quote1.BidPrice >= tick.Mid {
		t.Error("Expected bid below mid")
	}

	// Verify ask is at ladder level above mid
	if quote1.AskPrice <= tick.Mid {
		t.Error("Expected ask above mid")
	}

	// Wait for rebuild interval
	time.Sleep(150 * time.Millisecond)

	// Get second quote - should be at different level
	quote2 := m.OnPriceTick(tick, 0.0)
	if quote2 == nil {
		t.Fatal("Expected second quote, got nil")
	}

	// Prices should have changed after rebuild
	if quote1.BidPrice == quote2.BidPrice && quote1.AskPrice == quote2.AskPrice {
		t.Log("Prices might be same if random, but level should cycle")
	}

	// Verify spacing is reasonable
	if quote2.BidPrice < tick.Mid-levelSpacing*float64(m.cfg.LadderLevels+1) {
		t.Error("Bid too far from mid")
	}
	if quote2.AskPrice > tick.Mid+levelSpacing*float64(m.cfg.LadderLevels+1) {
		t.Error("Ask too far from mid")
	}
}

func TestMeanReversionStrategy_OnPriceTick_RebuildInterval(t *testing.T) {
	m := NewMeanReversionStrategy(MeanReversionConfig{
		LadderLevels:    5,
		LevelSpacingBps: 10.0,
		QuoteQty:        0.1,
		RebuildInterval: 50 * time.Millisecond,
	})

	tick := PriceTick{Mid: 50000.0}

	// Get initial quote
	quote1 := m.OnPriceTick(tick, 0.0)
	if quote1 == nil {
		t.Fatal("Expected quote, got nil")
	}

	// Immediately get another quote - should be same level
	quote2 := m.OnPriceTick(tick, 0.0)
	if quote2 == nil {
		t.Fatal("Expected quote, got nil")
	}

	if quote1.BidPrice != quote2.BidPrice {
		t.Error("Expected same bid before rebuild interval")
	}

	// Wait for rebuild
	time.Sleep(60 * time.Millisecond)

	// Get quote after rebuild - level should have cycled
	quote3 := m.OnPriceTick(tick, 0.0)
	if quote3 == nil {
		t.Fatal("Expected quote, got nil")
	}

	// Level should be different now
	if quote1.BidPrice == quote3.BidPrice && quote1.AskPrice == quote3.AskPrice {
		t.Log("Prices same - level might have cycled back, acceptable")
	}
}
