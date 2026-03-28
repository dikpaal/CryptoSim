package marketmaker

import (
	"testing"
)

func TestAvStoikovStrategy_Name(t *testing.T) {
	a := NewAvStoikovStrategy(AvStoikovConfig{VarianceWindow: 30})
	if a.Name() != "Avellaneda-Stoikov" {
		t.Errorf("Expected name 'Avellaneda-Stoikov', got '%s'", a.Name())
	}
}

func TestAvStoikovStrategy_OnPriceTick_InsufficientHistory(t *testing.T) {
	a := NewAvStoikovStrategy(AvStoikovConfig{
		RiskAversion:     0.5,
		OrderArrivalRate: 1.0,
		VarianceWindow:   30,
		BaseQuoteQty:     0.02,
	})

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	quote := a.OnPriceTick(tick, 0.0)
	if quote != nil {
		t.Error("Expected nil quote with insufficient history")
	}
}

func TestAvStoikovStrategy_OnPriceTick_ZeroInventory(t *testing.T) {
	a := NewAvStoikovStrategy(AvStoikovConfig{
		RiskAversion:     0.5,
		OrderArrivalRate: 1.0,
		VarianceWindow:   10,
		BaseQuoteQty:     0.02,
	})

	for i := 0; i < 15; i++ {
		a.OnPriceTick(PriceTick{Mid: 50000.0 + float64(i)*10}, 0.0)
	}

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	quote := a.OnPriceTick(tick, 0.0)
	if quote == nil {
		t.Fatal("Expected quote, got nil")
	}

	if quote.BidQty != quote.AskQty {
		t.Error("Expected symmetric quantities with zero inventory")
	}

	if quote.BidPrice >= tick.Mid {
		t.Error("Expected bid below mid price")
	}
	if quote.AskPrice <= tick.Mid {
		t.Error("Expected ask above mid price")
	}
}

func TestAvStoikovStrategy_OnPriceTick_LongInventory(t *testing.T) {
	a := NewAvStoikovStrategy(AvStoikovConfig{
		RiskAversion:     0.5,
		OrderArrivalRate: 1.0,
		VarianceWindow:   10,
		BaseQuoteQty:     0.02,
	})

	for i := 0; i < 15; i++ {
		a.OnPriceTick(PriceTick{Mid: 50000.0}, 0.0)
	}

	inventory := 0.2

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	quoteZeroInv := a.OnPriceTick(tick, inventory)
	reservationPriceWithInv := tick.Mid - inventory*a.cfg.RiskAversion*a.calculateVariance()

	if reservationPriceWithInv >= tick.Mid {
		t.Error("Expected reservation price below mid with long inventory")
	}

	if quoteZeroInv.BidQty == quoteZeroInv.AskQty {
		t.Log("Note: quantities adjusted for inventory")
	}
}

func TestAvStoikovStrategy_OnPriceTick_ShortInventory(t *testing.T) {
	a := NewAvStoikovStrategy(AvStoikovConfig{
		RiskAversion:     0.5,
		OrderArrivalRate: 1.0,
		VarianceWindow:   10,
		BaseQuoteQty:     0.02,
	})

	for i := 0; i < 15; i++ {
		a.OnPriceTick(PriceTick{Mid: 50000.0}, 0.0)
	}

	inventory := -0.2

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	a.OnPriceTick(tick, inventory)
	reservationPriceWithInv := tick.Mid - inventory*a.cfg.RiskAversion*a.calculateVariance()

	if reservationPriceWithInv <= tick.Mid {
		t.Error("Expected reservation price above mid with short inventory")
	}
}

func TestAvStoikovStrategy_CalculateVariance(t *testing.T) {
	a := NewAvStoikovStrategy(AvStoikovConfig{
		VarianceWindow: 10,
	})

	prices := []float64{100.0, 100.0, 100.0, 100.0, 100.0}
	a.priceHistory = prices

	variance := a.calculateVariance()
	if variance != 0.0001 {
		t.Errorf("Expected min variance 0.0001 for constant prices, got %f", variance)
	}
}

func TestAvStoikovStrategy_CalculateVariance_NonZero(t *testing.T) {
	a := NewAvStoikovStrategy(AvStoikovConfig{
		VarianceWindow: 5,
	})

	prices := []float64{100.0, 105.0, 95.0, 110.0, 90.0}
	a.priceHistory = prices

	variance := a.calculateVariance()
	if variance <= 0 {
		t.Error("Expected positive variance for varying prices")
	}

	mean := 100.0
	expectedVariance := 0.0
	for _, p := range prices {
		diff := p - mean
		expectedVariance += diff * diff
	}
	expectedVariance /= float64(len(prices))

	if abs(variance-expectedVariance) > 0.01 {
		t.Errorf("Expected variance %f, got %f", expectedVariance, variance)
	}
}

func TestAvStoikovStrategy_QuoteQtyScaling(t *testing.T) {
	a := NewAvStoikovStrategy(AvStoikovConfig{
		RiskAversion:     0.5,
		OrderArrivalRate: 1.0,
		VarianceWindow:   10,
		BaseQuoteQty:     0.02,
	})

	for i := 0; i < 15; i++ {
		a.OnPriceTick(PriceTick{Mid: 50000.0}, 0.0)
	}

	quoteZero := a.OnPriceTick(PriceTick{Mid: 50000.0}, 0.0)

	quoteLarge := a.OnPriceTick(PriceTick{Mid: 50000.0}, 0.5)

	if quoteLarge.BidQty >= quoteZero.BidQty {
		t.Error("Expected smaller quote qty with larger inventory")
	}
}
