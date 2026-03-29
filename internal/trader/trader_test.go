package trader

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testNATSURL = "nats://localhost:4222"

func TestNewTraderService(t *testing.T) {
	tests := []struct {
		name    string
		nc      *nats.Conn
		cfg     Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil nats connection",
			nc:      nil,
			cfg:     Config{},
			wantErr: true,
			errMsg:  "nats connection is nil",
		},
		{
			name:    "empty trader id",
			nc:      &nats.Conn{},
			cfg:     Config{ID: "", Symbol: "BTC-USD", OrdersPerSec: 10, MaxInFlight: 10},
			wantErr: true,
			errMsg:  "trader id is required",
		},
		{
			name:    "empty symbol",
			nc:      &nats.Conn{},
			cfg:     Config{ID: "trader-1", Symbol: "", OrdersPerSec: 10, MaxInFlight: 10},
			wantErr: true,
			errMsg:  "symbol is required",
		},
		{
			name:    "zero orders per sec",
			nc:      &nats.Conn{},
			cfg:     Config{ID: "trader-1", Symbol: "BTC-USD", OrdersPerSec: 0, MaxInFlight: 10},
			wantErr: true,
			errMsg:  "orders_per_sec must be > 0",
		},
		{
			name:    "zero max in flight",
			nc:      &nats.Conn{},
			cfg:     Config{ID: "trader-1", Symbol: "BTC-USD", OrdersPerSec: 10, MaxInFlight: 0},
			wantErr: true,
			errMsg:  "max_in_flight must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trader, err := NewTraderService(tt.nc, tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, trader)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, trader)
			}
		})
	}
}

func TestNewTraderService_ValidConfig(t *testing.T) {
	nc, err := nats.Connect(testNATSURL)
	if err != nil {
		t.Skipf("Skipping test: NATS not available at %s", testNATSURL)
	}
	defer nc.Close()

	cfg := Config{
		ID:                 "test-trader",
		Symbol:             "BTC-USD",
		OrdersPerSec:       100,
		MarketOrderRatio:   0.3,
		BuyRatio:           0.5,
		MinQty:             0.01,
		MaxQty:             0.1,
		AggressiveLimitBps: 5,
		MaxInFlight:        10,
		RequestTimeout:     250 * time.Millisecond,
		MatchFriendly:      true,
		PHitMM:             0.6,
	}

	trader, err := NewTraderService(nc, cfg)
	require.NoError(t, err)
	require.NotNil(t, trader)

	assert.Equal(t, cfg.ID, trader.cfg.ID)
	assert.Equal(t, cfg.Symbol, trader.cfg.Symbol)
	assert.Equal(t, DefaultSubmitTopic, trader.submitTopic)
	assert.Equal(t, DefaultCancelTopic, trader.cancelTopic)
	assert.NotNil(t, trader.rng)
	assert.Equal(t, time.Second/time.Duration(cfg.OrdersPerSec), trader.interval)
}

func TestTrader_GenerateOrder(t *testing.T) {
	nc, err := nats.Connect(testNATSURL)
	if err != nil {
		t.Skipf("Skipping test: NATS not available at %s", testNATSURL)
	}
	defer nc.Close()

	cfg := Config{
		ID:               "test-trader",
		Symbol:           "BTC-USD",
		OrdersPerSec:     100,
		MarketOrderRatio: 0.3,
		BuyRatio:         0.5,
		MinQty:           0.01,
		MaxQty:           0.1,
		MaxInFlight:      10,
		RequestTimeout:   250 * time.Millisecond,
	}

	trader, err := NewTraderService(nc, cfg)
	require.NoError(t, err)

	// Generate multiple orders and verify distribution
	marketOrders := 0
	limitOrders := 0
	buyOrders := 0
	sellOrders := 0

	numOrders := 1000
	for i := 0; i < numOrders; i++ {
		order := trader.generateOrder()

		// Verify basic fields
		assert.NotEmpty(t, order.ClientOrderID)
		assert.Equal(t, cfg.ID, order.MMID)
		assert.Equal(t, cfg.Symbol, order.Symbol)
		assert.True(t, order.Qty >= cfg.MinQty && order.Qty <= cfg.MaxQty)

		// Count order types
		if order.Type == "MARKET" {
			marketOrders++
			assert.Equal(t, 0.0, order.Price)
		} else {
			limitOrders++
			assert.Greater(t, order.Price, 0.0)
		}

		// Count sides
		if order.Side == "BID" {
			buyOrders++
		} else {
			sellOrders++
		}
	}

	// Verify distributions are roughly correct (within 10% tolerance)
	expectedMarket := int(float64(numOrders) * cfg.MarketOrderRatio)
	assert.InDelta(t, expectedMarket, marketOrders, float64(numOrders)*0.1)

	expectedBuy := int(float64(numOrders) * cfg.BuyRatio)
	assert.InDelta(t, expectedBuy, buyOrders, float64(numOrders)*0.1)
}

func TestTrader_SubmitOrderIntegration(t *testing.T) {
	nc, err := nats.Connect(testNATSURL)
	if err != nil {
		t.Skipf("Skipping test: NATS not available at %s", testNATSURL)
	}
	defer nc.Close()

	// Set up mock engine responder
	var mu sync.Mutex
	receivedOrders := []OrderSubmitRequest{}

	_, err = nc.QueueSubscribe(DefaultSubmitTopic, "test-engine", func(msg *nats.Msg) {
		var req OrderSubmitRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		mu.Lock()
		receivedOrders = append(receivedOrders, req)
		mu.Unlock()

		// Send mock reply
		reply := OrderSubmitReply{
			ClientOrderID: req.ClientOrderID,
			OrderID:       "mock-order-id",
			Accepted:      true,
			Status:        "PENDING",
		}
		replyData, _ := json.Marshal(reply)
		msg.Respond(replyData)
	})
	require.NoError(t, err)

	// Create trader
	cfg := Config{
		ID:             "test-trader",
		Symbol:         "BTC-USD",
		OrdersPerSec:   10,
		MinQty:         0.01,
		MaxQty:         0.1,
		MaxInFlight:    5,
		RequestTimeout: 500 * time.Millisecond,
	}

	trader, err := NewTraderService(nc, cfg)
	require.NoError(t, err)

	// Submit a few orders manually
	for i := 0; i < 5; i++ {
		trader.submitOrder()
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Verify orders were received
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 5, len(receivedOrders))

	// Verify stats
	stats := trader.SnapshotStats()
	assert.Equal(t, uint64(5), stats.Submitted)
	assert.Equal(t, uint64(5), stats.AckedAccepted)
	assert.Equal(t, uint64(0), stats.AckedRejected)
}

func TestTrader_RunAndStop(t *testing.T) {
	nc, err := nats.Connect(testNATSURL)
	if err != nil {
		t.Skipf("Skipping test: NATS not available at %s", testNATSURL)
	}
	defer nc.Close()

	// Set up mock engine responder
	var orderCount int64
	_, err = nc.QueueSubscribe(DefaultSubmitTopic, "test-engine", func(msg *nats.Msg) {
		var req OrderSubmitRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		orderCount++

		reply := OrderSubmitReply{
			ClientOrderID: req.ClientOrderID,
			OrderID:       "mock-order-id",
			Accepted:      true,
			Status:        "PENDING",
		}
		replyData, _ := json.Marshal(reply)
		msg.Respond(replyData)
	})
	require.NoError(t, err)

	// Create trader with 10 orders/sec
	cfg := Config{
		ID:             "test-trader",
		Symbol:         "BTC-USD",
		OrdersPerSec:   10,
		MinQty:         0.01,
		MaxQty:         0.1,
		MaxInFlight:    5,
		RequestTimeout: 500 * time.Millisecond,
	}

	trader, err := NewTraderService(nc, cfg)
	require.NoError(t, err)

	// Run trader in background
	go trader.Run()

	// Let it run for ~500ms (should submit ~5 orders)
	time.Sleep(500 * time.Millisecond)

	// Stop trader
	trader.Stop()

	// Wait a bit more to ensure shutdown completes
	time.Sleep(100 * time.Millisecond)

	// Verify orders were submitted
	stats := trader.SnapshotStats()
	assert.Greater(t, stats.Submitted, uint64(3)) // At least 3 orders
	assert.Less(t, stats.Submitted, uint64(10))   // But not too many
}

func TestTrader_MatchFriendlyMode(t *testing.T) {
	nc, err := nats.Connect(testNATSURL)
	if err != nil {
		t.Skipf("Skipping test: NATS not available at %s", testNATSURL)
	}
	defer nc.Close()

	cfg := Config{
		ID:                 "test-trader",
		Symbol:             "BTC-USD",
		OrdersPerSec:       10,
		MinQty:             0.01,
		MaxQty:             0.1,
		MaxInFlight:        5,
		RequestTimeout:     500 * time.Millisecond,
		MatchFriendly:      true,
		AggressiveLimitBps: 10,
	}

	trader, err := NewTraderService(nc, cfg)
	require.NoError(t, err)

	// Inject mock orderbook snapshot
	trader.lastSnapshot = &OrderbookSnapshot{
		Bids: [][2]float64{{50000.0, 0.5}, {49999.0, 0.3}},
		Asks: [][2]float64{{50001.0, 0.4}, {50002.0, 0.6}},
	}

	// Generate orders and verify they're marketable
	for i := 0; i < 10; i++ {
		order := trader.generateOrder()

		if order.Type == "LIMIT" {
			// With AggressiveLimitBps=10, buy orders should cross ask, sell orders should cross bid
			if order.Side == "BID" {
				assert.Greater(t, order.Price, 50001.0, "Buy limit order should be above best ask")
			} else {
				assert.Less(t, order.Price, 50000.0, "Sell limit order should be below best bid")
			}
		}
	}
}

func TestTrader_MaxInFlightEnforcement(t *testing.T) {
	nc, err := nats.Connect(testNATSURL)
	if err != nil {
		t.Skipf("Skipping test: NATS not available at %s", testNATSURL)
	}
	defer nc.Close()

	// Set up slow mock engine responder
	var concurrentRequests int32
	var maxConcurrent int32
	var mu sync.Mutex

	_, err = nc.QueueSubscribe(DefaultSubmitTopic, "test-engine", func(msg *nats.Msg) {
		mu.Lock()
		concurrentRequests++
		if concurrentRequests > maxConcurrent {
			maxConcurrent = concurrentRequests
		}
		mu.Unlock()

		// Simulate slow processing
		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		concurrentRequests--
		mu.Unlock()

		var req OrderSubmitRequest
		json.Unmarshal(msg.Data, &req)
		reply := OrderSubmitReply{
			ClientOrderID: req.ClientOrderID,
			OrderID:       "mock-order-id",
			Accepted:      true,
			Status:        "PENDING",
		}
		replyData, _ := json.Marshal(reply)
		msg.Respond(replyData)
	})
	require.NoError(t, err)

	// Create trader with MaxInFlight=3
	cfg := Config{
		ID:             "test-trader",
		Symbol:         "BTC-USD",
		OrdersPerSec:   50, // High rate to test concurrency
		MinQty:         0.01,
		MaxQty:         0.1,
		MaxInFlight:    3,
		RequestTimeout: 500 * time.Millisecond,
	}

	trader, err := NewTraderService(nc, cfg)
	require.NoError(t, err)

	// Run trader
	go trader.Run()

	// Let it run for a bit
	time.Sleep(300 * time.Millisecond)

	// Stop trader
	trader.Stop()

	// Verify max concurrent requests didn't exceed MaxInFlight
	mu.Lock()
	defer mu.Unlock()
	assert.LessOrEqual(t, maxConcurrent, int32(cfg.MaxInFlight))
}
