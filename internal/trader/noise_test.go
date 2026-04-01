package trader

import (
	"testing"
	"time"
)

func TestNoiseStrategy_Name(t *testing.T) {
	n := NewNoiseStrategy(NoiseConfig{})
	if n.Name() != "Noise" {
		t.Errorf("Expected name 'Noise', got '%s'", n.Name())
	}
}

func TestNoiseStrategy_OnPriceTick_FirstTick(t *testing.T) {
	n := NewNoiseStrategy(NoiseConfig{
		MinQty:         0.01,
		MaxQty:         0.1,
		MinIntervalMs:  100,
		MaxIntervalMs:  200,
		MarketOrderPct: 50.0,
		MaxSpreadBps:   10.0,
	})

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	// First tick should initialize but not fire
	quote := n.OnPriceTick(tick, 0.0)
	if quote != nil {
		t.Error("Expected nil quote on first tick")
	}

	if n.lastFireTime.IsZero() {
		t.Error("Expected lastFireTime to be set")
	}
}

func TestNoiseStrategy_OnPriceTick_IntervalTiming(t *testing.T) {
	n := NewNoiseStrategy(NoiseConfig{
		MinQty:         0.01,
		MaxQty:         0.1,
		MinIntervalMs:  50,
		MaxIntervalMs:  100,
		MarketOrderPct: 0.0, // No market orders for predictability
		MaxSpreadBps:   10.0,
	})

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	// Initialize
	n.OnPriceTick(tick, 0.0)

	// Immediate second tick should return nil
	quote := n.OnPriceTick(tick, 0.0)
	if quote != nil {
		t.Error("Expected nil quote before interval elapses")
	}

	// Wait for max interval + buffer
	time.Sleep(150 * time.Millisecond)

	// Should fire now
	quote = n.OnPriceTick(tick, 0.0)
	if quote == nil {
		t.Error("Expected quote after interval elapsed")
	}
}

func TestNoiseStrategy_OnPriceTick_RandomBuyOrSell(t *testing.T) {
	n := NewNoiseStrategy(NoiseConfig{
		MinQty:         0.01,
		MaxQty:         0.1,
		MinIntervalMs:  10,
		MaxIntervalMs:  20,
		MarketOrderPct: 0.0,
		MaxSpreadBps:   10.0,
	})

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    50000.0,
		Ask:    50010.0,
		Mid:    50005.0,
	}

	// Initialize
	n.OnPriceTick(tick, 0.0)

	seenBuy := false
	seenSell := false

	// Run multiple iterations to see both buy and sell
	for i := 0; i < 100; i++ {
		time.Sleep(25 * time.Millisecond)
		quote := n.OnPriceTick(tick, 0.0)
		if quote == nil {
			continue
		}

		// Either bid or ask should be set, not both
		if quote.BidQty > 0 && quote.AskQty > 0 {
			t.Error("Expected either bid or ask, not both")
		}

		if quote.BidQty > 0 {
			seenBuy = true
			// Verify quantity in range
			if quote.BidQty < 0.01 || quote.BidQty > 0.1 {
				t.Errorf("Bid qty %f out of range [0.01, 0.1]", quote.BidQty)
			}
			// Verify price is reasonable (limit order)
			if quote.BidPrice > tick.Mid {
				t.Error("Limit buy should be below or at mid")
			}
		}

		if quote.AskQty > 0 {
			seenSell = true
			// Verify quantity in range
			if quote.AskQty < 0.01 || quote.AskQty > 0.1 {
				t.Errorf("Ask qty %f out of range [0.01, 0.1]", quote.AskQty)
			}
			// Verify price is reasonable (limit order)
			if quote.AskPrice < tick.Mid {
				t.Error("Limit sell should be above or at mid")
			}
		}

		if seenBuy && seenSell {
			break
		}
	}

	if !seenBuy || !seenSell {
		t.Log("Warning: didn't observe both buy and sell in 100 iterations")
	}
}

func TestNoiseStrategy_OnPriceTick_QuantityRange(t *testing.T) {
	n := NewNoiseStrategy(NoiseConfig{
		MinQty:         0.05,
		MaxQty:         0.15,
		MinIntervalMs:  10,
		MaxIntervalMs:  20,
		MarketOrderPct: 0.0,
		MaxSpreadBps:   10.0,
	})

	tick := PriceTick{Mid: 50000.0, Bid: 49995.0, Ask: 50005.0}

	n.OnPriceTick(tick, 0.0)

	for i := 0; i < 50; i++ {
		time.Sleep(25 * time.Millisecond)
		quote := n.OnPriceTick(tick, 0.0)
		if quote == nil {
			continue
		}

		qty := quote.BidQty
		if qty == 0 {
			qty = quote.AskQty
		}

		if qty < 0.05 || qty > 0.15 {
			t.Errorf("Quantity %f outside configured range [0.05, 0.15]", qty)
		}
	}
}

func TestNoiseStrategy_RandomInterval(t *testing.T) {
	n := NewNoiseStrategy(NoiseConfig{
		MinIntervalMs: 100,
		MaxIntervalMs: 200,
	})

	for i := 0; i < 10; i++ {
		interval := n.randomInterval()
		ms := interval.Milliseconds()

		if ms < 100 || ms > 200 {
			t.Errorf("Interval %d ms outside range [100, 200]", ms)
		}
	}
}
