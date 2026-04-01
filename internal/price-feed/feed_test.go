package pricefeed

import (
	"cryptosim/internal/models"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
)

func TestPriceFeedPublishing(t *testing.T) {
	// Step 1: Create fake Coinbase WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection: %v", err)
			return
		}
		defer conn.Close()

		// Wait for subscription message
		var subMsg PriceFeedRequest
		conn.ReadJSON(&subMsg)

		// Send fake ticker response for all three symbols
		mockTicker := TickerResponse{
			Channel:   "ticker",
			Timestamp: time.Now().Format(time.RFC3339),
			Events: []Event{
				{
					Type: "ticker",
					Tickers: []Ticker{
						{
							ProductID: "BTC-USD",
							BestBid:   "50000.00",
							BestAsk:   "50001.00",
							Price:     "50000.50",
							Volume24H: "1000",
							Low24H:    "49000",
							High24H:   "51000",
						},
						{
							ProductID: "ETH-USD",
							BestBid:   "3000.00",
							BestAsk:   "3001.00",
							Price:     "3000.50",
							Volume24H: "5000",
							Low24H:    "2900",
							High24H:   "3100",
						},
						{
							ProductID: "XRP-USD",
							BestBid:   "0.50",
							BestAsk:   "0.51",
							Price:     "0.505",
							Volume24H: "10000",
							Low24H:    "0.48",
							High24H:   "0.52",
						},
					},
				},
			},
		}
		conn.WriteJSON(mockTicker)

		// Keep connection alive
		time.Sleep(3 * time.Second)
	}))
	defer server.Close()

	// Step 2: Connect to real NATS and subscribe
	natsConn, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer natsConn.Close()

	received := make(chan PriceTick, 1)

	_, err = natsConn.Subscribe(PricesLiveTopic, func(msg *nats.Msg) {
		var pd PriceTick
		json.Unmarshal(msg.Data, &pd)
		received <- pd
	})
	if err != nil {
		t.Fatalf("Failed to subscribe to NATS: %v", err)
	}

	// Step 3: Create and start PriceFeedService
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	pfs := &PriceFeedService{}
	pfs.natsConn, err = NewNATSConn("nats://localhost:4222")
	if err != nil {
		t.Fatalf("Failed to create NATS connection: %v", err)
	}
	defer pfs.natsConn.Close()

	pfs.wsConn, _, err = pfs.dial(wsURL)
	if err != nil {
		t.Fatalf("Failed to dial WebSocket: %v", err)
	}
	defer pfs.wsConn.Close()

	// Subscribe to the ticker channel for all three symbols
	pfs.subscribe(pfs.wsConn, "subscribe", []models.ProductId{models.BTC_USD, models.ETH_USD, models.XRP_USD}, "ticker")

	// Start publishing in background
	go pfs.startLivePricesPublisher()

	// Step 4: Wait for all three messages and assert
	expectedPrices := map[string]struct{ bid, ask float64 }{
		"BTC-USD": {50000.00, 50001.00},
		"ETH-USD": {3000.00, 3001.00},
		"XRP-USD": {0.50, 0.51},
	}

	receivedCount := 0
	timeout := time.After(3 * time.Second)

	for receivedCount < 3 {
		select {
		case priceTick := <-received:
			expected, ok := expectedPrices[priceTick.Symbol]
			if !ok {
				t.Errorf("Unexpected symbol: %s", priceTick.Symbol)
			}
			if priceTick.Bid != expected.bid {
				t.Errorf("Expected bid %f for %s, got %f", expected.bid, priceTick.Symbol, priceTick.Bid)
			}
			if priceTick.Ask != expected.ask {
				t.Errorf("Expected ask %f for %s, got %f", expected.ask, priceTick.Symbol, priceTick.Ask)
			}
			t.Logf("Successfully received price data for %s: %+v", priceTick.Symbol, priceTick)
			receivedCount++
		case <-timeout:
			t.Fatalf("Timeout waiting for price data. Received %d/3 symbols", receivedCount)
		}
	}
}
