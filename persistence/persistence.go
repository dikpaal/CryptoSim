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

type PersistenceService struct {
	n              *NATSConn
	tradeChan      chan models.Trade
	batchBuffer    []models.Trade
	circularBuffer *models.CircularBuffer
	db             *pgxpool.Pool // need DB connection
	mu             sync.Mutex
}

func NewPersistenceService(url string, db *pgxpool.Pool) (*PersistenceService, error) {

	n, err := NewNATSConn(url)
	if err != nil {
		return nil, err
	}

	return &PersistenceService{
		n:              n,
		tradeChan:      make(chan models.Trade, 1000),
		batchBuffer:    make([]models.Trade, 0, 100),
		circularBuffer: models.NewCircularBuffer(10000),
		db:             db,
	}, nil
}

func (persistenceService *PersistenceService) SubscribeTradesExecuted() {
	go persistenceService.worker(persistenceService.tradeChan) // start once, not per message

	persistenceService.n.nc.Subscribe(TradesExecutedTopic, func(m *nats.Msg) {
		var trade models.Trade
		json.Unmarshal(m.Data, &trade)
		persistenceService.tradeChan <- trade // just send
	})
}

func (persistenceService *PersistenceService) worker(buffer <-chan models.Trade) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case trade := <-buffer:
			persistenceService.mu.Lock()
			persistenceService.batchBuffer = append(persistenceService.batchBuffer, trade)
			shouldFlush := len(persistenceService.batchBuffer) >= 100
			persistenceService.mu.Unlock()

			if shouldFlush {
				persistenceService.FlushAll()
			}

		case <-ticker.C:
			persistenceService.FlushAll()
		}
	}
}

func (persistenceService *PersistenceService) FlushAll() {
	persistenceService.mu.Lock()
	if len(persistenceService.batchBuffer) == 0 {
		persistenceService.mu.Unlock()
		return
	}

	trades := make([]models.Trade, len(persistenceService.batchBuffer))
	copy(trades, persistenceService.batchBuffer)
	persistenceService.batchBuffer = persistenceService.batchBuffer[:0] // clear, keep capacity
	persistenceService.mu.Unlock()

	// Try to write to DB
	err := persistenceService.writeTradesToDB(trades)
	if err != nil {
		log.Printf("DB write failed, adding to circular buffer: %v", err)
		for _, trade := range trades {
			persistenceService.circularBuffer.Add(trade)
		}
		return
	}

	// If DB is back up and circular buffer has data, flush!!!!!!
	if persistenceService.circularBuffer.Len() > 0 {
		bufferedTrades := persistenceService.circularBuffer.FlushAll()
		if err := persistenceService.writeTradesToDB(bufferedTrades); err != nil {
			log.Printf("Failed to flush circular buffer: %v", err)
			// Put them back in circular buffer
			for _, trade := range bufferedTrades {
				persistenceService.circularBuffer.Add(trade)
			}
		}
	}
}

func (persistenceService *PersistenceService) writeTradesToDB(trades []models.Trade) error {
	ctx := context.Background()

	_, err := persistenceService.db.CopyFrom(
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
