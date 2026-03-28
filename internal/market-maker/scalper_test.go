package marketmaker

import (
	"testing"
)

func TestScalperStrategy_Name(t *testing.T) {
	s := NewScalperStrategy(ScalperConfig{})
	if s.Name() != "Scalper" {
		t.Errorf("Expected name 'Scalper', got '%s'", s.Name())
	}
}

func TestScalperStrategy_OnPriceTick_SymmetricQuotes(t *testing.T) {
	s := NewScalperStrategy(ScalperConfig{
		SpreadBps:              5.0,
		QuoteQty:               0.01,
		InventorySkewThreshold: 0.1,
	})

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	quote := s.OnPriceTick(tick, 0.0)

	if quote == nil {
		t.Fatal("Expected quote, got nil")
	}

	expectedSpread := 50005.0 * (5.0 / 10000.0)
	expectedBid := 50005.0 - expectedSpread/2
	expectedAsk := 50005.0 + expectedSpread/2

	if abs(quote.BidPrice-expectedBid) > 0.01 {
		t.Errorf("Expected bid %f, got %f", expectedBid, quote.BidPrice)
	}
	if abs(quote.AskPrice-expectedAsk) > 0.01 {
		t.Errorf("Expected ask %f, got %f", expectedAsk, quote.AskPrice)
	}
	if quote.BidQty != 0.01 {
		t.Errorf("Expected bid qty 0.01, got %f", quote.BidQty)
	}
	if quote.AskQty != 0.01 {
		t.Errorf("Expected ask qty 0.01, got %f", quote.AskQty)
	}
}

func TestScalperStrategy_OnPriceTick_LongInventorySkew(t *testing.T) {
	s := NewScalperStrategy(ScalperConfig{
		SpreadBps:              5.0,
		QuoteQty:               0.01,
		InventorySkewThreshold: 0.1,
	})

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	quote := s.OnPriceTick(tick, 0.15)

	if quote == nil {
		t.Fatal("Expected quote, got nil")
	}

	if quote.AskQty <= quote.BidQty {
		t.Error("Expected ask qty > bid qty when long inventory")
	}

	if quote.AskPrice >= tick.Mid {
		t.Error("Expected tighter ask when long inventory")
	}
}

func TestScalperStrategy_OnPriceTick_ShortInventorySkew(t *testing.T) {
	s := NewScalperStrategy(ScalperConfig{
		SpreadBps:              5.0,
		QuoteQty:               0.01,
		InventorySkewThreshold: 0.1,
	})

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	quote := s.OnPriceTick(tick, -0.15)

	if quote == nil {
		t.Fatal("Expected quote, got nil")
	}

	if quote.BidQty <= quote.AskQty {
		t.Error("Expected bid qty > ask qty when short inventory")
	}

	if quote.BidPrice <= tick.Mid {
		t.Error("Expected tighter bid when short inventory")
	}
}
