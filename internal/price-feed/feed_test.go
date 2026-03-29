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

		// Send fake ticker response
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

	_, err = natsConn.Subscribe(models.PricesLiveTopic, func(msg *nats.Msg) {
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

	// Subscribe to the ticker channel
	pfs.subscribe(pfs.wsConn, "subscribe", []models.ProductId{models.BTC_USD}, "ticker")

	// Start publishing in background
	go pfs.startLivePricesPublisher()

	// Step 4: Wait for message and assert
	select {
	case priceTick := <-received:
		if priceTick.Symbol != "BTC-USD" {
			t.Errorf("Expected symbol BTC-USD, got %s", priceTick.Symbol)
		}
		if priceTick.Bid != 50000.00 {
			t.Errorf("Expected bid 50000.00, got %f", priceTick.Bid)
		}
		if priceTick.Ask != 50001.00 {
			t.Errorf("Expected ask 50001.00, got %f", priceTick.Ask)
		}
		t.Logf("Successfully received price data: %+v", priceTick)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for price data on NATS")
	}
}
