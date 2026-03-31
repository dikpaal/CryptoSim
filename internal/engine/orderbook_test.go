package engine

import (
	"cryptosim/internal/models"
	"testing"
)

func TestLimitOrderMatching(t *testing.T) {
	ob := NewOrderBook("BTC-USD")

	buyOrder := models.NewOrder("mm1", "BTC-USD", models.Bid, models.Limit, 50000, 1.0)
	trades := ob.SubmitOrder(buyOrder)

	if len(trades) != 0 {
		t.Errorf("Expected no trades, got %d", len(trades))
	}

	if buyOrder.Status != models.Pending {
		t.Errorf("Expected order status PENDING, got %s", buyOrder.Status)
	}

	sellOrder := models.NewOrder("mm2", "BTC-USD", models.Ask, models.Limit, 49000, 0.5)
	trades = ob.SubmitOrder(sellOrder)

	if len(trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(trades))
	}

	trade := trades[0]
	if trade.Price != 50000 {
		t.Errorf("Expected trade price 50000, got %f", trade.Price)
	}

	if trade.Qty != 0.5 {
		t.Errorf("Expected trade qty 0.5, got %f", trade.Qty)
	}

	if trade.BuyerID != "mm1" || trade.SellerID != "mm2" {
		t.Errorf("Expected buyer mm1 and seller mm2, got %s and %s", trade.BuyerID, trade.SellerID)
	}

	if sellOrder.Status != models.Filled {
		t.Errorf("Expected sell order status FILLED, got %s", sellOrder.Status)
	}

	if buyOrder.Status != models.Partial {
		t.Errorf("Expected buy order status PARTIAL, got %s", buyOrder.Status)
	}
}

func TestPartialFills(t *testing.T) {
	ob := NewOrderBook("BTC-USD")

	buyOrder := models.NewOrder("mm1", "BTC-USD", models.Bid, models.Limit, 50000, 1.0)
	ob.SubmitOrder(buyOrder)

	sellOrder1 := models.NewOrder("mm2", "BTC-USD", models.Ask, models.Limit, 49000, 0.5)
	trades1 := ob.SubmitOrder(sellOrder1)

	if len(trades1) != 1 {
		t.Fatalf("Expected 1 trade after first sell, got %d", len(trades1))
	}

	if buyOrder.RemainingQty() != 0.5 {
		t.Errorf("Expected remaining qty 0.5, got %f", buyOrder.RemainingQty())
	}

	sellOrder2 := models.NewOrder("mm3", "BTC-USD", models.Ask, models.Limit, 48000, 0.2)
	trades2 := ob.SubmitOrder(sellOrder2)

	if len(trades2) != 1 {
		t.Fatalf("Expected 1 trade after second sell, got %d", len(trades2))
	}

	remaining := buyOrder.RemainingQty()
	if remaining < 0.29 || remaining > 0.31 {
		t.Errorf("Expected remaining qty ~0.3, got %f", remaining)
	}

	if buyOrder.Status != models.Partial {
		t.Errorf("Expected buy order status PARTIAL, got %s", buyOrder.Status)
	}
}

func TestMarketOrder(t *testing.T) {
	ob := NewOrderBook("BTC-USD")

	buyOrder := models.NewOrder("mm1", "BTC-USD", models.Bid, models.Limit, 50000, 1.0)
	ob.SubmitOrder(buyOrder)

	marketSell := models.NewOrder("mm2", "BTC-USD", models.Ask, models.Market, 0, 0.5)
	trades := ob.SubmitOrder(marketSell)

	if len(trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(trades))
	}

	trade := trades[0]
	if trade.Price != 50000 {
		t.Errorf("Expected trade at best bid price 50000, got %f", trade.Price)
	}

	if trade.Qty != 0.5 {
		t.Errorf("Expected trade qty 0.5, got %f", trade.Qty)
	}

	if marketSell.Status != models.Filled {
		t.Errorf("Expected market order status FILLED, got %s", marketSell.Status)
	}
}

func TestMarketOrderCancellation(t *testing.T) {
	ob := NewOrderBook("BTC-USD")

	marketBuy := models.NewOrder("mm1", "BTC-USD", models.Bid, models.Market, 0, 1.0)
	trades := ob.SubmitOrder(marketBuy)

	if len(trades) != 0 {
		t.Errorf("Expected no trades on empty book, got %d", len(trades))
	}

	if marketBuy.Status != models.Cancelled {
		t.Errorf("Expected unfilled market order to be CANCELLED, got %s", marketBuy.Status)
	}
}

func TestPriceTimePriority(t *testing.T) {
	ob := NewOrderBook("BTC-USD")

	buy1 := models.NewOrder("mm1", "BTC-USD", models.Bid, models.Limit, 50000, 0.5)
	buy2 := models.NewOrder("mm2", "BTC-USD", models.Bid, models.Limit, 50000, 0.3)
	buy3 := models.NewOrder("mm3", "BTC-USD", models.Bid, models.Limit, 49000, 0.2)

	ob.SubmitOrder(buy1)
	ob.SubmitOrder(buy2)
	ob.SubmitOrder(buy3)

	sellOrder := models.NewOrder("mm4", "BTC-USD", models.Ask, models.Limit, 49000, 1.0)
	trades := ob.SubmitOrder(sellOrder)

	if len(trades) != 3 {
		t.Fatalf("Expected 3 trades, got %d", len(trades))
	}

	if trades[0].BuyerOrderID != buy1.ID {
		t.Errorf("First trade should match buy1 (best price, first in time)")
	}

	if trades[1].BuyerOrderID != buy2.ID {
		t.Errorf("Second trade should match buy2 (best price, second in time)")
	}

	if trades[2].BuyerOrderID != buy3.ID {
		t.Errorf("Third trade should match buy3 (lower price)")
	}

	if trades[0].Qty != 0.5 {
		t.Errorf("Expected first trade qty 0.5, got %f", trades[0].Qty)
	}

	if trades[1].Qty != 0.3 {
		t.Errorf("Expected second trade qty 0.3, got %f", trades[1].Qty)
	}

	qty3 := trades[2].Qty
	if qty3 < 0.19 || qty3 > 0.21 {
		t.Errorf("Expected third trade qty ~0.2, got %f", qty3)
	}
}

func TestCancelOrder(t *testing.T) {
	ob := NewOrderBook("BTC-USD")

	order := models.NewOrder("mm1", "BTC-USD", models.Bid, models.Limit, 50000, 1.0)
	ob.SubmitOrder(order)

	success := ob.CancelOrder(order.ID)
	if !success {
		t.Error("Expected cancel to succeed")
	}

	_, exists := ob.GetOrder(order.ID)
	if exists {
		t.Error("Order should not exist after cancellation")
	}

	success = ob.CancelOrder(order.ID)
	if success {
		t.Error("Expected cancel of non-existent order to fail")
	}
}

func TestGetSnapshot(t *testing.T) {
	ob := NewOrderBook("BTC-USD")

	ob.SubmitOrder(models.NewOrder("mm1", "BTC-USD", models.Bid, models.Limit, 50000, 1.0))
	ob.SubmitOrder(models.NewOrder("mm2", "BTC-USD", models.Bid, models.Limit, 49000, 0.5))
	ob.SubmitOrder(models.NewOrder("mm3", "BTC-USD", models.Ask, models.Limit, 51000, 0.3))
	ob.SubmitOrder(models.NewOrder("mm4", "BTC-USD", models.Ask, models.Limit, 52000, 0.2))

	asks, bids := ob.GetSnapshot(10)

	if len(bids) != 2 {
		t.Errorf("Expected 2 bids, got %d", len(bids))
	}

	if len(asks) != 2 {
		t.Errorf("Expected 2 asks, got %d", len(asks))
	}

	if bids[0][0] != 50000 {
		t.Errorf("Best bid should be 50000, got %f", bids[0][0])
	}

	if asks[0][0] != 51000 {
		t.Errorf("Best ask should be 51000, got %f", asks[0][0])
	}
}
