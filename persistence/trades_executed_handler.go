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
}

func NewTradesHandler(url string, db *pgxpool.Pool) (*TradesHandler, error) {

	n, err := NewNATSConn(url)
	if err != nil {
		return nil, err
	}

	return &TradesHandler{
		n:              n,
		tradeChan:      make(chan models.Trade, 5000),
		batchBuffer:    make([]models.Trade, 0, 2500),
		circularBuffer: models.NewCircularBuffer(15000),
		db:             db,
	}, nil
}

func (tradesHandler *TradesHandler) SubscribeTradesExecuted() {
	go tradesHandler.worker(tradesHandler.tradeChan) // start once, not per message

	tradesHandler.n.nc.Subscribe(TradesExecutedTopic, func(m *nats.Msg) {
		var trade models.Trade
		json.Unmarshal(m.Data, &trade)
		tradesHandler.tradeChan <- trade // just send
	})
}

func (tradesHandler *TradesHandler) worker(buffer <-chan models.Trade) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case trade := <-buffer:
			tradesHandler.mu.Lock()
			tradesHandler.batchBuffer = append(tradesHandler.batchBuffer, trade)
			shouldFlush := len(tradesHandler.batchBuffer) >= 100
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
				trades[i].BuyerMMID,
				trades[i].SellerMMID,
				trades[i].BuyerOrderID,
				trades[i].SellerOrderID,
				trades[i].ExecutedAt,
			}, nil
		}),
	)

	return err
}
