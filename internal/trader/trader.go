package trader

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

const (
	// DefaultSubmitSubject is the NATS subject for order submission (request-reply).
	DefaultSubmitSubject = "orders.submit"
	// DefaultCancelSubject is the NATS subject for order cancellation (request-reply).
	DefaultCancelSubject = "orders.cancel"
)

type Config struct {
	// ID identifies this trader instance (used as mm_id in submitted orders).
	ID string
	// Symbol is the instrument this trader targets (e.g. "BTC-USD").
	Symbol string

	// OrdersPerSec is the target submission rate (before backpressure).
	OrdersPerSec int
	// MarketOrderRatio is the fraction of orders that are MARKET vs LIMIT.
	MarketOrderRatio float64
	// BuyRatio is the fraction of orders that are buys (BID) vs sells (ASK).
	BuyRatio float64
	// MinQty is the minimum order size.
	MinQty float64
	// MaxQty is the maximum order size.
	MaxQty float64
	// AggressiveLimitBps optionally makes LIMIT orders marketable by crossing top-of-book by N bps.
	// (0 means don't skew; exact interpretation is up to the trader implementation.)
	AggressiveLimitBps float64

	// MaxInFlight caps concurrent outstanding request-reply submissions.
	MaxInFlight int
	// RequestTimeout is the per-request timeout for NATS request-reply.
	RequestTimeout time.Duration

	// MatchFriendly enables "always try to match" order generation (e.g., marketable orders).
	MatchFriendly bool
	// PHitMM biases flow toward trading against MM liquidity (if trader uses snapshots/tape).
	PHitMM float64

	// SubmitSubject overrides the NATS subject used for order submission.
	SubmitSubject string
	// CancelSubject overrides the NATS subject used for order cancellation.
	CancelSubject string
}

type Stats struct {
	// Submitted counts submit attempts (requests sent).
	Submitted uint64
	// AckedAccepted counts acks that indicate the order was accepted.
	AckedAccepted uint64
	// AckedRejected counts acks that indicate the order was rejected.
	AckedRejected uint64
	// Timeouts counts request-reply timeouts.
	Timeouts uint64
	// Errors counts other submission/encoding/transport errors.
	Errors uint64
}

type Trader struct {
	// cfg holds immutable config knobs for this trader.
	cfg Config

	// nc is the underlying NATS connection used for request-reply.
	nc *nats.Conn
	// submitSubject is the resolved submit subject (cfg override or default).
	submitSubject string
	// cancelSubject is the resolved cancel subject (cfg override or default).
	cancelSubject string

	// interval is derived from OrdersPerSec (time.Second / OrdersPerSec).
	interval time.Duration

	// sem enforces MaxInFlight by bounding concurrent request goroutines.
	sem chan struct{}
	// ctx/cancel controls shutdown for background loops.
	ctx    context.Context
	cancel context.CancelFunc

	// rng for order generation randomization.
	rng *rand.Rand

	// Optional: cached orderbook snapshot for match-friendly mode.
	lastSnapshot *OrderbookSnapshot

	// Counters for basic benchmarking/visibility.
	submitted     atomic.Uint64
	ackedAccepted atomic.Uint64
	ackedRejected atomic.Uint64
	timeouts      atomic.Uint64
	errors        atomic.Uint64
}

// OrderbookSnapshot holds cached top-of-book data.
type OrderbookSnapshot struct {
	Bids [][2]float64 `json:"bids"`
	Asks [][2]float64 `json:"asks"`
}

// OrderSubmitRequest matches engine NATS message format.
type OrderSubmitRequest struct {
	ClientOrderID string  `json:"client_order_id"`
	MMID          string  `json:"mm_id"`
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	Type          string  `json:"type"`
	Price         float64 `json:"price"`
	Qty           float64 `json:"qty"`
	Timestamp     int64   `json:"timestamp"`
}

// OrderSubmitReply matches engine NATS reply format.
type OrderSubmitReply struct {
	ClientOrderID string `json:"client_order_id"`
	OrderID       string `json:"order_id"`
	Accepted      bool   `json:"accepted"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
}

func NewTraderService(nc *nats.Conn, cfg Config) (*Trader, error) {
	if nc == nil {
		return nil, errors.New("nats connection is nil")
	}
	if cfg.ID == "" {
		return nil, errors.New("trader id is required")
	}
	if cfg.Symbol == "" {
		return nil, errors.New("symbol is required")
	}
	if cfg.OrdersPerSec <= 0 {
		return nil, errors.New("orders_per_sec must be > 0")
	}
	if cfg.MaxInFlight <= 0 {
		return nil, errors.New("max_in_flight must be > 0")
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 250 * time.Millisecond
	}
	if cfg.SubmitSubject == "" {
		cfg.SubmitSubject = DefaultSubmitSubject
	}
	if cfg.CancelSubject == "" {
		cfg.CancelSubject = DefaultCancelSubject
	}

	interval := time.Second / time.Duration(cfg.OrdersPerSec)
	ctx, cancel := context.WithCancel(context.Background())

	return &Trader{
		cfg: cfg,

		nc:            nc,
		submitSubject: cfg.SubmitSubject,
		cancelSubject: cfg.CancelSubject,

		interval: interval,

		sem:    make(chan struct{}, cfg.MaxInFlight),
		ctx:    ctx,
		cancel: cancel,

		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func (t *Trader) Stop() {
	if t != nil && t.cancel != nil {
		t.cancel()
	}
}

func (t *Trader) SnapshotStats() Stats {
	return Stats{
		Submitted:     t.submitted.Load(),
		AckedAccepted: t.ackedAccepted.Load(),
		AckedRejected: t.ackedRejected.Load(),
		Timeouts:      t.timeouts.Load(),
		Errors:        t.errors.Load(),
	}
}

func (t *Trader) Run() {
	log.Printf("Trader %s starting: %d orders/sec, symbol=%s", t.cfg.ID, t.cfg.OrdersPerSec, t.cfg.Symbol)

	// If match-friendly mode, optionally subscribe to orderbook snapshots
	if t.cfg.MatchFriendly {
		t.subscribeToOrderbookSnapshots()
	}

	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			log.Printf("Trader %s stopping", t.cfg.ID)
			return
		case <-ticker.C:
			go t.submitOrder()
		}
	}
}

// subscribes to orderbook.snapshot and caches the latest.
func (t *Trader) subscribeToOrderbookSnapshots() {
	_, err := t.nc.Subscribe("orderbook.snapshot", func(msg *nats.Msg) {
		var snapshot OrderbookSnapshot
		if err := json.Unmarshal(msg.Data, &snapshot); err != nil {
			log.Printf("Error unmarshaling orderbook snapshot: %v", err)
			return
		}
		t.lastSnapshot = &snapshot
	})
	if err != nil {
		log.Printf("Warning: failed to subscribe to orderbook.snapshot: %v", err)
	} else {
		log.Printf("Trader %s subscribed to orderbook.snapshot for match-friendly mode", t.cfg.ID)
	}
}

// generates and submits a single order via NATS request-reply.
func (t *Trader) submitOrder() {
	// Acquire semaphore slot (enforce MaxInFlight)
	select {
	case t.sem <- struct{}{}:
		defer func() { <-t.sem }()
	case <-t.ctx.Done():
		return
	}

	order := t.generateOrder()

	reqData, err := json.Marshal(order)
	if err != nil {
		t.errors.Add(1)
		log.Printf("Error marshaling order: %v", err)
		return
	}

	t.submitted.Add(1)
	msg, err := t.nc.Request(t.submitSubject, reqData, t.cfg.RequestTimeout)
	if err != nil {
		if err == nats.ErrTimeout {
			t.timeouts.Add(1)
		} else {
			t.errors.Add(1)
		}
		return
	}

	var reply OrderSubmitReply
	if err := json.Unmarshal(msg.Data, &reply); err != nil {
		t.errors.Add(1)
		log.Printf("Error unmarshaling reply: %v", err)
		return
	}

	if reply.Accepted {
		t.ackedAccepted.Add(1)
	} else {
		t.ackedRejected.Add(1)
	}
}

// creates a random order based on trader config.
func (t *Trader) generateOrder() OrderSubmitRequest {
	isMarket := t.rng.Float64() < t.cfg.MarketOrderRatio
	orderType := "LIMIT"
	if isMarket {
		orderType = "MARKET"
	}

	isBuy := t.rng.Float64() < t.cfg.BuyRatio
	side := "BID"
	if !isBuy {
		side = "ASK"
	}

	qty := t.cfg.MinQty + t.rng.Float64()*(t.cfg.MaxQty-t.cfg.MinQty) // Generate quantity (uniform between MinQty and MaxQty)
	price := t.generatePrice(side, isMarket)

	return OrderSubmitRequest{
		ClientOrderID: uuid.New().String(),
		MMID:          t.cfg.ID,
		Symbol:        t.cfg.Symbol,
		Side:          side,
		Type:          orderType,
		Price:         price,
		Qty:           qty,
		Timestamp:     time.Now().UnixNano(),
	}
}

// generates an order price based on side, order type, and config.
func (t *Trader) generatePrice(side string, isMarket bool) float64 {
	// Market orders have price = 0
	if isMarket {
		return 0
	}

	// If match-friendly mode and we have a snapshot, bias toward crossing spread
	if t.cfg.MatchFriendly && t.lastSnapshot != nil && len(t.lastSnapshot.Bids) > 0 && len(t.lastSnapshot.Asks) > 0 {
		bestBid := t.lastSnapshot.Bids[0][0]
		bestAsk := t.lastSnapshot.Asks[0][0]
		mid := (bestBid + bestAsk) / 2

		// If AggressiveLimitBps is set, make orders marketable by crossing the spread
		if t.cfg.AggressiveLimitBps > 0 {
			offset := mid * (t.cfg.AggressiveLimitBps / 10000.0)
			if side == "BID" {
				// Aggressive buy: bid above current best ask
				return bestAsk + offset
			} else {
				// Aggressive sell: ask below current best bid
				return bestBid - offset
			}
		}

		// Otherwise, use mid-price with small random offset
		randomOffset := t.rng.Float64()*mid*0.001 - mid*0.0005
		return mid + randomOffset
	}

	// Fallback: generate price around a baseline (e.g., $50,000 for BTC-USD)
	baseline := 50000.0
	randomOffset := t.rng.Float64()*baseline*0.01 - baseline*0.005
	return baseline + randomOffset
}
