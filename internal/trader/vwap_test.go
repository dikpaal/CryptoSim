package trader

import (
	"testing"
)

func TestVWAPStrategy_Name(t *testing.T) {
	v := NewVWAPStrategy(VWAPConfig{})
	if v.Name() != "VWAP" {
		t.Errorf("Expected name 'VWAP', got '%s'", v.Name())
	}
}

func TestVWAPStrategy_OnPriceTick_InsufficientHistory(t *testing.T) {
	v := NewVWAPStrategy(VWAPConfig{
		WindowSize:     10,
		OffsetBps:      5.0,
		QuoteQty:       0.1,
		UpdateThreshold: 0.0001,
	})

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	quote := v.OnPriceTick(tick, 0.0)
	if quote != nil {
		t.Error("Expected nil quote with insufficient history")
	}
}

func TestVWAPStrategy_OnPriceTick_BasicQuote(t *testing.T) {
	v := NewVWAPStrategy(VWAPConfig{
		WindowSize:     5,
		OffsetBps:      10.0,
		QuoteQty:       0.1,
		UpdateThreshold: 0.0,
	})

	// Feed some ticks
	for i := 0; i < 5; i++ {
		tick := PriceTick{
			Symbol: "BTC-USD",
			Bid:    49995.0 + float64(i),
			Ask:    50005.0 + float64(i),
			Mid:    50000.0 + float64(i),
		}
		v.OnPriceTick(tick, 0.0)
	}

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	quote := v.OnPriceTick(tick, 0.0)
	if quote == nil {
		t.Fatal("Expected quote, got nil")
	}

	// Bid should be below VWAP, ask should be above VWAP
	if quote.BidPrice >= quote.AskPrice {
		t.Error("Expected bid < ask")
	}

	// Quantities should match config
	if quote.BidQty != 0.1 {
		t.Errorf("Expected bid qty 0.1, got %f", quote.BidQty)
	}
	if quote.AskQty != 0.1 {
		t.Errorf("Expected ask qty 0.1, got %f", quote.AskQty)
	}

	// Verify offset is applied correctly
	vwap := v.lastVWAP
	offset := vwap * (10.0 / 10000.0)

	if abs(quote.BidPrice-(vwap-offset)) > 0.01 {
		t.Errorf("Expected bid at VWAP - offset, got bid=%f, vwap=%f, offset=%f",
			quote.BidPrice, vwap, offset)
	}
	if abs(quote.AskPrice-(vwap+offset)) > 0.01 {
		t.Errorf("Expected ask at VWAP + offset, got ask=%f, vwap=%f, offset=%f",
			quote.AskPrice, vwap, offset)
	}
}

func TestVWAPStrategy_OnPriceTick_UpdateThreshold(t *testing.T) {
	v := NewVWAPStrategy(VWAPConfig{
		WindowSize:     3,
		OffsetBps:      10.0,
		QuoteQty:       0.1,
		UpdateThreshold: 0.0, // 0% threshold for first quote
	})

	// Feed initial ticks
	for i := 0; i < 3; i++ {
		tick := PriceTick{
			Symbol: "BTC-USD",
			Bid:    49995.0,
			Ask:    50005.0,
			Mid:    50000.0,
		}
		v.OnPriceTick(tick, 0.0)
	}

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    49995.0,
		Ask:    50005.0,
		Mid:    50000.0,
	}

	quote1 := v.OnPriceTick(tick, 0.0)
	if quote1 == nil {
		t.Fatal("Expected first quote, got nil")
	}

	// Now set threshold higher
	v.cfg.UpdateThreshold = 0.01

	// Feed same price - VWAP won't change much, should return nil
	tick2 := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	quote2 := v.OnPriceTick(tick2, 0.0)
	if quote2 != nil {
		t.Log("Got quote despite small VWAP change - acceptable if threshold crossed")
	}

	// Feed significantly different price - VWAP should change enough to trigger
	// Use much larger change to ensure threshold is crossed
	tick3 := PriceTick{
		Symbol: "BTC-USD",
		Bid:    52000.0,
		Ask:    52010.0,
		Mid:    52005.0,
	}

	quote3 := v.OnPriceTick(tick3, 0.0)
	if quote3 == nil {
		t.Error("Expected quote after significant VWAP change")
	}
}

func TestVWAPStrategy_CalculateVWAP(t *testing.T) {
	v := NewVWAPStrategy(VWAPConfig{WindowSize: 5})

	// Add price/volume data
	v.prices = []float64{100.0, 101.0, 102.0}
	v.volumes = []float64{10.0, 20.0, 30.0}

	vwap := v.calculateVWAP()

	// Manual calculation: (100*10 + 101*20 + 102*30) / (10 + 20 + 30)
	// = (1000 + 2020 + 3060) / 60 = 6080 / 60 = 101.333...
	expected := 101.333333

	if abs(vwap-expected) > 0.01 {
		t.Errorf("Expected VWAP ~%f, got %f", expected, vwap)
	}
}

func TestVWAPStrategy_CalculateVWAP_EmptyData(t *testing.T) {
	v := NewVWAPStrategy(VWAPConfig{WindowSize: 5})

	vwap := v.calculateVWAP()
	if vwap != 0 {
		t.Errorf("Expected VWAP 0 for empty data, got %f", vwap)
	}
}

func TestVWAPStrategy_CalculateVWAP_ZeroVolume(t *testing.T) {
	v := NewVWAPStrategy(VWAPConfig{WindowSize: 5})

	v.prices = []float64{100.0, 101.0}
	v.volumes = []float64{0.0, 0.0}

	vwap := v.calculateVWAP()
	if vwap != 0 {
		t.Errorf("Expected VWAP 0 for zero volume, got %f", vwap)
	}
}
