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

type OrderbookSnapshotHandler struct {
	n              *NATSConn
	snapshotChan   chan models.OrderbookSnapshot
	batchBuffer    []models.OrderbookSnapshot
	circularBuffer *models.CircularBufferSnapshot
	db             *pgxpool.Pool
	mu             sync.Mutex
	lastSnapshot   time.Time
}

func NewOrderbookSnapshotHandler(url string, db *pgxpool.Pool) (*OrderbookSnapshotHandler, error) {
	n, err := NewNATSConn(url)
	if err != nil {
		return nil, err
	}

	return &OrderbookSnapshotHandler{
		n:              n,
		snapshotChan:   make(chan models.OrderbookSnapshot, 1000),
		batchBuffer:    make([]models.OrderbookSnapshot, 0, 100),
		circularBuffer: models.NewCircularBufferSnapshot(10000),
		db:             db,
	}, nil
}

func (h *OrderbookSnapshotHandler) Subscribe() {
	go h.worker(h.snapshotChan)

	h.n.nc.Subscribe(OrderBookSnapshotTopic, func(m *nats.Msg) {
		var raw map[string]interface{}
		json.Unmarshal(m.Data, &raw)

		h.mu.Lock()
		if time.Since(h.lastSnapshot) < 1*time.Second {
			h.mu.Unlock()
			return
		}
		h.lastSnapshot = time.Now()
		h.mu.Unlock()

		snapshot := models.OrderbookSnapshot{
			Symbol:     raw["symbol"].(string),
			Bids:       convertToFloat64Array(raw["bids"]),
			Asks:       convertToFloat64Array(raw["asks"]),
			SnapshotAt: time.Now(),
		}

		h.snapshotChan <- snapshot
	})
}

func (h *OrderbookSnapshotHandler) worker(buffer <-chan models.OrderbookSnapshot) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case snapshot := <-buffer:
			h.mu.Lock()
			h.batchBuffer = append(h.batchBuffer, snapshot)
			shouldFlush := len(h.batchBuffer) >= 100
			h.mu.Unlock()

			if shouldFlush {
				h.FlushAll()
			}

		case <-ticker.C:
			h.FlushAll()
		}
	}
}

func (h *OrderbookSnapshotHandler) FlushAll() {
	h.mu.Lock()
	if len(h.batchBuffer) == 0 {
		h.mu.Unlock()
		return
	}

	snapshots := make([]models.OrderbookSnapshot, len(h.batchBuffer))
	copy(snapshots, h.batchBuffer)
	h.batchBuffer = h.batchBuffer[:0]
	h.mu.Unlock()

	err := h.writeSnapshotsToDb(snapshots)
	if err != nil {
		log.Printf("DB write failed, adding to circular buffer: %v", err)
		for _, snapshot := range snapshots {
			h.circularBuffer.Add(snapshot)
		}
		return
	}

	if h.circularBuffer.Len() > 0 {
		buffered := h.circularBuffer.FlushAll()
		if err := h.writeSnapshotsToDb(buffered); err != nil {
			log.Printf("Failed to flush circular buffer: %v", err)
			for _, snapshot := range buffered {
				h.circularBuffer.Add(snapshot)
			}
		}
	}
}

func (h *OrderbookSnapshotHandler) writeSnapshotsToDb(snapshots []models.OrderbookSnapshot) error {
	ctx := context.Background()

	_, err := h.db.CopyFrom(
		ctx,
		pgx.Identifier{"orderbook_snapshots"},
		[]string{"symbol", "bids", "asks", "mid_price", "spread", "snapshot_at"},
		pgx.CopyFromSlice(len(snapshots), func(i int) ([]any, error) {
			snapshot := snapshots[i]

			midPrice, spread := calculateMidPriceAndSpread(snapshot.Bids, snapshot.Asks)

			bidsJSON, _ := json.Marshal(snapshot.Bids)
			asksJSON, _ := json.Marshal(snapshot.Asks)

			return []any{
				snapshot.Symbol,
				bidsJSON,
				asksJSON,
				midPrice,
				spread,
				snapshot.SnapshotAt,
			}, nil
		}),
	)

	return err
}

func calculateMidPriceAndSpread(bids, asks [][2]float64) (float64, float64) {
	if len(bids) == 0 || len(asks) == 0 {
		return 0, 0
	}
	bestBid := bids[0][0]
	bestAsk := asks[0][0]
	return (bestBid + bestAsk) / 2, bestAsk - bestBid
}

func convertToFloat64Array(raw interface{}) [][2]float64 {
	arr := raw.([]interface{})
	result := make([][2]float64, len(arr))
	for i, v := range arr {
		inner := v.([]interface{})
		result[i] = [2]float64{inner[0].(float64), inner[1].(float64)}
	}
	return result
}
