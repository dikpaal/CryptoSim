package marketmaker

import (
	"cryptosim/internal/models"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

type MockStrategy struct {
	name        string
	quoteToReturn *Quote
	tradesCalled  int
	ticksCalled   int
}

func (m *MockStrategy) Name() string {
	return m.name
}

func (m *MockStrategy) OnPriceTick(tick PriceTick, inventory float64) *Quote {
	m.ticksCalled++
	return m.quoteToReturn
}

func (m *MockStrategy) OnTrade(trade *models.Trade) {
	m.tradesCalled++
}

func TestNewMarketMaker(t *testing.T) {
	mockStrategy := &MockStrategy{name: "TestStrategy"}
	cfg := Config{
		ID:           "mm-test",
		Symbol:       "BTC-USD",
		MaxInventory: 1.0,
		MaxOrders:    10,
		Strategy:     mockStrategy,
	}

	mm := NewMarketMaker(nil, cfg)

	if mm == nil {
		t.Fatal("Expected market maker instance, got nil")
	}

	if mm.cfg.ID != "mm-test" {
		t.Errorf("Expected ID 'mm-test', got '%s'", mm.cfg.ID)
	}

	if mm.cfg.Symbol != "BTC-USD" {
		t.Errorf("Expected symbol 'BTC-USD', got '%s'", mm.cfg.Symbol)
	}

	if mm.strategy != mockStrategy {
		t.Error("Expected strategy to be set")
	}

	if mm.activeOrders == nil {
		t.Error("Expected activeOrders map to be initialized")
	}
}

func TestMarketMaker_HandlePriceTick(t *testing.T) {
	mockStrategy := &MockStrategy{
		name: "TestStrategy",
		quoteToReturn: &Quote{
			BidPrice: 49995.0,
			BidQty:   0.01,
			AskPrice: 50005.0,
			AskQty:   0.01,
		},
	}

	cfg := Config{
		ID:           "mm-test",
		Symbol:       "BTC-USD",
		MaxInventory: 1.0,
		MaxOrders:    10,
		Strategy:     mockStrategy,
	}

	mm := NewMarketMaker(nil, cfg)

	tick := PriceTick{
		Symbol: "BTC-USD",
		Bid:    49990.0,
		Ask:    50010.0,
		Mid:    50000.0,
	}

	tickData, _ := json.Marshal(tick)
	msg := &nats.Msg{Data: tickData}

	mm.handlePriceTick(msg)

	mm.mu.RLock()
	currentMid := mm.currentMid
	mm.mu.RUnlock()

	if currentMid != 50000.0 {
		t.Errorf("Expected currentMid 50000.0, got %f", currentMid)
	}

	if mockStrategy.ticksCalled != 1 {
		t.Errorf("Expected 1 tick call, got %d", mockStrategy.ticksCalled)
	}
}

func TestMarketMaker_HandlePriceTick_NilQuote(t *testing.T) {
	mockStrategy := &MockStrategy{
		name:          "TestStrategy",
		quoteToReturn: nil,
	}

	cfg := Config{
		ID:           "mm-test",
		Symbol:       "BTC-USD",
		MaxInventory: 1.0,
		MaxOrders:    10,
		Strategy:     mockStrategy,
	}

	mm := NewMarketMaker(nil, cfg)

	tick := PriceTick{
		Symbol: "BTC-USD",
		Mid:    50000.0,
	}

	tickData, _ := json.Marshal(tick)
	msg := &nats.Msg{Data: tickData}

	mm.handlePriceTick(msg)

	if mockStrategy.ticksCalled != 1 {
		t.Errorf("Expected 1 tick call, got %d", mockStrategy.ticksCalled)
	}
}

func TestMarketMaker_HandlePriceTick_InventoryLimit(t *testing.T) {
	mockStrategy := &MockStrategy{
		name: "TestStrategy",
		quoteToReturn: &Quote{
			BidPrice: 49995.0,
			BidQty:   0.01,
			AskPrice: 50005.0,
			AskQty:   0.01,
		},
	}

	cfg := Config{
		ID:           "mm-test",
		Symbol:       "BTC-USD",
		MaxInventory: 0.5,
		MaxOrders:    10,
		Strategy:     mockStrategy,
	}

	mm := NewMarketMaker(nil, cfg)
	mm.inventory = 0.6

	tick := PriceTick{
		Symbol: "BTC-USD",
		Mid:    50000.0,
	}

	tickData, _ := json.Marshal(tick)
	msg := &nats.Msg{Data: tickData}

	mm.handlePriceTick(msg)

	if mockStrategy.ticksCalled != 1 {
		t.Errorf("Expected 1 tick call, got %d", mockStrategy.ticksCalled)
	}
}

func TestMarketMaker_HandleTradeExecuted_Buyer(t *testing.T) {
	mockStrategy := &MockStrategy{name: "TestStrategy"}

	cfg := Config{
		ID:           "mm-test",
		Symbol:       "BTC-USD",
		MaxInventory: 1.0,
		MaxOrders:    10,
		Strategy:     mockStrategy,
	}

	mm := NewMarketMaker(nil, cfg)
	mm.avgCost = 50000.0

	trade := models.Trade{
		BuyerMMID:     "mm-test",
		SellerMMID:    "mm-other",
		Price:         50100.0,
		Qty:           0.05,
		BuyerOrderID:  "order-123",
		SellerOrderID: "order-456",
	}

	tradeData, _ := json.Marshal(trade)
	msg := &nats.Msg{Data: tradeData}

	mm.activeOrders["order-123"] = true

	mm.handleTradeExecuted(msg)

	mm.mu.RLock()
	inventory := mm.inventory
	_, orderExists := mm.activeOrders["order-123"]
	mm.mu.RUnlock()

	if inventory != 0.05 {
		t.Errorf("Expected inventory 0.05, got %f", inventory)
	}

	if orderExists {
		t.Error("Expected order to be removed from activeOrders")
	}

	if mockStrategy.tradesCalled != 1 {
		t.Errorf("Expected 1 trade call, got %d", mockStrategy.tradesCalled)
	}
}

func TestMarketMaker_HandleTradeExecuted_Seller(t *testing.T) {
	mockStrategy := &MockStrategy{name: "TestStrategy"}

	cfg := Config{
		ID:           "mm-test",
		Symbol:       "BTC-USD",
		MaxInventory: 1.0,
		MaxOrders:    10,
		Strategy:     mockStrategy,
	}

	mm := NewMarketMaker(nil, cfg)
	mm.avgCost = 50000.0

	trade := models.Trade{
		BuyerMMID:     "mm-other",
		SellerMMID:    "mm-test",
		Price:         50100.0,
		Qty:           0.03,
		BuyerOrderID:  "order-123",
		SellerOrderID: "order-456",
	}

	tradeData, _ := json.Marshal(trade)
	msg := &nats.Msg{Data: tradeData}

	mm.activeOrders["order-456"] = true

	mm.handleTradeExecuted(msg)

	mm.mu.RLock()
	inventory := mm.inventory
	_, orderExists := mm.activeOrders["order-456"]
	mm.mu.RUnlock()

	if inventory != -0.03 {
		t.Errorf("Expected inventory -0.03, got %f", inventory)
	}

	if orderExists {
		t.Error("Expected order to be removed from activeOrders")
	}

	if mockStrategy.tradesCalled != 1 {
		t.Errorf("Expected 1 trade call, got %d", mockStrategy.tradesCalled)
	}
}

func TestMarketMaker_HandleTradeExecuted_NotInvolved(t *testing.T) {
	mockStrategy := &MockStrategy{name: "TestStrategy"}

	cfg := Config{
		ID:           "mm-test",
		Symbol:       "BTC-USD",
		MaxInventory: 1.0,
		MaxOrders:    10,
		Strategy:     mockStrategy,
	}

	mm := NewMarketMaker(nil, cfg)

	trade := models.Trade{
		BuyerMMID:  "mm-other1",
		SellerMMID: "mm-other2",
		Price:      50100.0,
		Qty:        0.05,
	}

	tradeData, _ := json.Marshal(trade)
	msg := &nats.Msg{Data: tradeData}

	mm.handleTradeExecuted(msg)

	mm.mu.RLock()
	inventory := mm.inventory
	mm.mu.RUnlock()

	if inventory != 0.0 {
		t.Errorf("Expected inventory 0.0, got %f", inventory)
	}

	if mockStrategy.tradesCalled != 0 {
		t.Errorf("Expected 0 trade calls, got %d", mockStrategy.tradesCalled)
	}
}

func TestMarketMaker_Stop(t *testing.T) {
	mockStrategy := &MockStrategy{name: "TestStrategy"}

	cfg := Config{
		ID:           "mm-test",
		Symbol:       "BTC-USD",
		MaxInventory: 1.0,
		MaxOrders:    10,
		Strategy:     mockStrategy,
	}

	mm := NewMarketMaker(nil, cfg)

	mm.Stop()

	select {
	case <-mm.ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected context to be cancelled")
	}
}

func TestAbs(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{5.0, 5.0},
		{-5.0, 5.0},
		{0.0, 0.0},
		{-0.001, 0.001},
	}

	for _, test := range tests {
		result := abs(test.input)
		if result != test.expected {
			t.Errorf("abs(%f) = %f, expected %f", test.input, result, test.expected)
		}
	}
}

func TestSignChanged(t *testing.T) {
	tests := []struct {
		old      float64
		new      float64
		expected bool
	}{
		{1.0, -1.0, true},
		{-1.0, 1.0, true},
		{1.0, 2.0, false},
		{-1.0, -2.0, false},
		{0.0, 1.0, false},
		{1.0, 0.0, false},
	}

	for _, test := range tests {
		result := signChanged(test.old, test.new)
		if result != test.expected {
			t.Errorf("signChanged(%f, %f) = %t, expected %t", test.old, test.new, result, test.expected)
		}
	}
}

