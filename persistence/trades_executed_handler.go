package persistence

import (
	"context"
	"cryptosim/internal/models"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

type TradesHandler struct {
	n              *NATSConn
	tradeChan      chan models.Trade
	batchBuffer    []models.Trade
	circularBuffer *models.CircularBuffer
	db             *pgxpool.Pool // need DB connection
	mu             sync.Mutex
	writesCount    uint64
	lastLogTime    time.Time
}

func NewTradesHandler(url string, db *pgxpool.Pool) (*TradesHandler, error) {

	n, err := NewNATSConn(url)
	if err != nil {
		return nil, err
	}

	return &TradesHandler{
		n:              n,
		tradeChan:      make(chan models.Trade, 150000),
		batchBuffer:    make([]models.Trade, 0, 50000),
		circularBuffer: models.NewCircularBuffer(100000),
		db:             db,
		lastLogTime:    time.Now(),
	}, nil
}

func (tradesHandler *TradesHandler) SubscribeTradesExecuted() {

	for i := 0; i < 8; i++ { // Increased workers - DB can handle 57k/s
		go tradesHandler.worker(tradesHandler.tradeChan)
	}

	// Subscribe with increased pending limits to prevent slow consumer
	sub, err := tradesHandler.n.nc.Subscribe(TradesExecutedTopic, func(m *nats.Msg) {
		var trade models.Trade
		json.Unmarshal(m.Data, &trade)

		// Non-blocking send - drop if channel full (better than blocking)
		select {
		case tradesHandler.tradeChan <- trade:
		default:
			log.Println("WARNING: trade channel full, dropping message")
		}
	})

	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// Set pending limits to 2M messages / 2GB (way higher than default 65k)
	// At 50k trades/s, this gives ~40s buffer before slow consumer
	sub.SetPendingLimits(2000000, 2*1024*1024*1024)

	log.Printf("Subscribed to %s with pending limits: 2M msgs / 2GB", TradesExecutedTopic)
}

func (tradesHandler *TradesHandler) worker(buffer <-chan models.Trade) {
	// Faster flush interval - DB can handle 57k/s
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case trade := <-buffer:
			tradesHandler.mu.Lock()
			tradesHandler.batchBuffer = append(tradesHandler.batchBuffer, trade)
			// Smaller batch threshold for smoother throughput (5k instead of 10k)
			shouldFlush := len(tradesHandler.batchBuffer) >= 5000
			tradesHandler.mu.Unlock()

			if shouldFlush {
				tradesHandler.FlushAll()
			}

		case <-ticker.C:
			tradesHandler.FlushAll()
		}
	}
}

func (tradesHandler *TradesHandler) FlushAll() {
	tradesHandler.mu.Lock()
	if len(tradesHandler.batchBuffer) == 0 {
		tradesHandler.mu.Unlock()
		return
	}

	trades := make([]models.Trade, len(tradesHandler.batchBuffer))
	copy(trades, tradesHandler.batchBuffer)
	tradesHandler.batchBuffer = tradesHandler.batchBuffer[:0] // clear, keep capacity
	tradesHandler.mu.Unlock()

	// Try to write to DB
	err := tradesHandler.writeTradesToDB(trades)
	if err != nil {
		log.Printf("DB write failed, adding to circular buffer: %v", err)
		for _, trade := range trades {
			tradesHandler.circularBuffer.Add(trade)
		}
		return
	}

	// If DB is back up and circular buffer has data, flush!!!!!!
	if tradesHandler.circularBuffer.Len() > 0 {
		bufferedTrades := tradesHandler.circularBuffer.FlushAll()
		if err := tradesHandler.writeTradesToDB(bufferedTrades); err != nil {
			log.Printf("Failed to flush circular buffer: %v", err)
			// Put them back in circular buffer
			for _, trade := range bufferedTrades {
				tradesHandler.circularBuffer.Add(trade)
			}
		}
	}
}

func (tradesHandler *TradesHandler) writeTradesToDB(trades []models.Trade) error {
	ctx := context.Background()

	_, err := tradesHandler.db.CopyFrom(
		ctx,
		pgx.Identifier{"trades"},
		[]string{"trade_id", "symbol", "price", "qty", "buyer_mm_id", "seller_mm_id", "buyer_order_id",
			"seller_order_id", "executed_at"},
		pgx.CopyFromSlice(len(trades), func(i int) ([]any, error) {
			return []any{
				trades[i].TradeID,
				trades[i].Symbol,
				trades[i].Price,
				trades[i].Qty,
				trades[i].BuyerID,
				trades[i].SellerID,
				trades[i].BuyerOrderID,
				trades[i].SellerOrderID,
				trades[i].ExecutedAt,
			}, nil
		}),
	)

	if err == nil {
		tradesHandler.writesCount += uint64(len(trades))

		// Log writes/s every 5 seconds
		now := time.Now()
		elapsed := now.Sub(tradesHandler.lastLogTime).Seconds()
		if elapsed >= 5.0 {
			writesPerSec := float64(tradesHandler.writesCount) / elapsed
			log.Printf("DB writes: %d trades in %.1fs (%.1f writes/s)", tradesHandler.writesCount, elapsed, writesPerSec)
			tradesHandler.writesCount = 0
			tradesHandler.lastLogTime = now
		}
	}

	return err
}
