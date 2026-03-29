package trader

import (
	"context"
	"cryptosim/internal/models"
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
	DefaultSubmitTopic = "orders.submit" // topic for order submission (request-reply)
	DefaultCancelTopic = "orders.cancel" // topic for order cancellation (request-reply)
)

type Config struct {
	ID                 string
	Symbol             string
	OrdersPerSec       int           // target submission rate (before backpressure).
	MarketOrderRatio   float64       // fraction of orders that are MARKET vs LIMIT.
	BuyRatio           float64       // fraction of orders that are buys (BID) vs sells (ASK).
	MinQty             float64       // minimum order size
	MaxQty             float64       // maximum order size.
	AggressiveLimitBps float64       // makes LIMIT orders marketable by crossing top-of-book by N bps (0 means don't skew). Added this to address liquidity issues
	MaxInFlight        int           // max concurrent outstanding request-reply submissions.
	RequestTimeout     time.Duration // per-request timeout for NATS request-reply.
	MatchFriendly      bool          // enables "always try to match" order generation (e.g., marketable orders).
	PHitMM             float64       // biases flow toward trading against MM liquidity (if trader uses snapshots/tape).
	SubmitTopic        string        // overrides the NATS topic used for order submission.
	CancelTopic        string        // overrides the NATS topic used for order cancellation.
}

type Stats struct {
	Submitted     uint64 // submit attempts (requests sent).
	AckedAccepted uint64 // acks that indicate the order was accepted.
	AckedRejected uint64 // acks that indicate the order was rejected.
	Timeouts      uint64 // request-reply timeouts.
	Errors        uint64 // other submission/encoding/transport errors.
}

type Trader struct {
	nc           *nats.Conn
	cfg          Config             // immutable config knobs for this trader.
	submitTopic  string             // submit topic (cfg override or default).
	cancelTopic  string             // cancel topic (cfg override or default).
	interval     time.Duration      // (time.Second / OrdersPerSec).
	sem          chan struct{}      // to enforce MaxInFlight
	rng          *rand.Rand         // rng for order generation randomization.
	lastSnapshot *OrderbookSnapshot // cached orderbook snapshot for match-friendly mode.

	ctx    context.Context
	cancel context.CancelFunc

	// Counters for benchmarking/visibility.
	submitted     atomic.Uint64
	ackedAccepted atomic.Uint64
	ackedRejected atomic.Uint64
	timeouts      atomic.Uint64
	errors        atomic.Uint64
}

type OrderbookSnapshot struct {
	Bids [][2]float64 `json:"bids"`
	Asks [][2]float64 `json:"asks"`
}

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
	if cfg.SubmitTopic == "" {
		cfg.SubmitTopic = DefaultSubmitTopic
	}
	if cfg.CancelTopic == "" {
		cfg.CancelTopic = DefaultCancelTopic
	}

	interval := time.Second / time.Duration(cfg.OrdersPerSec)
	ctx, cancel := context.WithCancel(context.Background())

	return &Trader{
		cfg: cfg,

		nc:          nc,
		submitTopic: cfg.SubmitTopic,
		cancelTopic: cfg.CancelTopic,

		interval: interval,

		sem:    make(chan struct{}, cfg.MaxInFlight),
		ctx:    ctx,
		cancel: cancel,

		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func (trader *Trader) Stop() {
	if trader != nil && trader.cancel != nil {
		trader.cancel()
	}
}

func (trader *Trader) SnapshotStats() Stats {
	return Stats{
		Submitted:     trader.submitted.Load(),
		AckedAccepted: trader.ackedAccepted.Load(),
		AckedRejected: trader.ackedRejected.Load(),
		Timeouts:      trader.timeouts.Load(),
		Errors:        trader.errors.Load(),
	}
}

func (trader *Trader) Run() {
	log.Printf("Trader %s starting: %d orders/sec, symbol=%s", trader.cfg.ID, trader.cfg.OrdersPerSec, trader.cfg.Symbol)

	// If match-friendly mode, optionally subscribe to orderbook snapshots
	if trader.cfg.MatchFriendly {
		trader.subscribeToOrderbookSnapshots()
	}

	ticker := time.NewTicker(trader.interval)
	defer ticker.Stop()

	for {
		select {
		case <-trader.ctx.Done():
			log.Printf("Trader %s stopping", trader.cfg.ID)
			return
		case <-ticker.C:
			go trader.submitOrder()
		}
	}
}

func (trader *Trader) subscribeToOrderbookSnapshots() {
	_, err := trader.nc.Subscribe(models.OrderBookSnapshotTopic, func(msg *nats.Msg) {
		var snapshot OrderbookSnapshot
		err := json.Unmarshal(msg.Data, &snapshot)
		if err != nil {
			log.Printf("Error unmarshaling orderbook snapshot: %v", err)
			return
		}
		trader.lastSnapshot = &snapshot
	})
	if err != nil {
		log.Printf("Warning: failed to subscribe to orderbook.snapshot: %v", err)
	} else {
		log.Printf("Trader %s subscribed to orderbook.snapshot for match-friendly mode", trader.cfg.ID)
	}
}

func (trader *Trader) submitOrder() {
	select {
	case trader.sem <- struct{}{}:
		defer func() { <-trader.sem }()
	case <-trader.ctx.Done():
		return
	}

	order := trader.generateOrder()

	reqData, err := json.Marshal(order)
	if err != nil {
		trader.errors.Add(1)
		log.Printf("Error marshaling order: %v", err)
		return
	}

	trader.submitted.Add(1)
	msg, err := trader.nc.Request(trader.submitTopic, reqData, trader.cfg.RequestTimeout)
	if err != nil {
		if err == nats.ErrTimeout {
			trader.timeouts.Add(1)
		} else {
			trader.errors.Add(1)
		}
		return
	}

	var reply OrderSubmitReply
	if err := json.Unmarshal(msg.Data, &reply); err != nil {
		trader.errors.Add(1)
		log.Printf("Error unmarshaling reply: %v", err)
		return
	}

	if reply.Accepted {
		trader.ackedAccepted.Add(1)
	} else {
		trader.ackedRejected.Add(1)
	}
}

func (trader *Trader) generateOrder() OrderSubmitRequest {
	isMarket := trader.rng.Float64() < trader.cfg.MarketOrderRatio
	orderType := "LIMIT"
	if isMarket {
		orderType = "MARKET"
	}

	isBuy := trader.rng.Float64() < trader.cfg.BuyRatio
	side := "BID"
	if !isBuy {
		side = "ASK"
	}

	// min + rand*(max-min) is the standard way to pick a uniform random number between min and max
	qty := trader.cfg.MinQty + trader.rng.Float64()*(trader.cfg.MaxQty-trader.cfg.MinQty)
	price := trader.generatePrice(side, isMarket)

	return OrderSubmitRequest{
		ClientOrderID: uuid.New().String(),
		MMID:          trader.cfg.ID,
		Symbol:        trader.cfg.Symbol,
		Side:          side,
		Type:          orderType,
		Price:         price,
		Qty:           qty,
		Timestamp:     time.Now().UnixNano(),
	}
}

func (trader *Trader) generatePrice(side string, isMarket bool) float64 {
	if isMarket {
		return 0
	}

	if trader.cfg.MatchFriendly && trader.lastSnapshot != nil && len(trader.lastSnapshot.Bids) > 0 && len(trader.lastSnapshot.Asks) > 0 {
		bestBid := trader.lastSnapshot.Bids[0][0]
		bestAsk := trader.lastSnapshot.Asks[0][0]
		mid := (bestBid + bestAsk) / 2

		// If AggressiveLimitBps is set, make orders marketable by crossing the spread
		if trader.cfg.AggressiveLimitBps > 0 {
			offset := mid * (trader.cfg.AggressiveLimitBps / 10000.0)
			if side == "BID" {
				return bestAsk + offset
			} else {
				return bestBid - offset
			}
		}

		randomOffset := trader.rng.Float64()*mid*0.001 - mid*0.0005
		return mid + randomOffset
	}
	baseline := 50000.0
	randomOffset := trader.rng.Float64()*baseline*0.01 - baseline*0.005
	return baseline + randomOffset
}
