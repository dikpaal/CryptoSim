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

	"github.com/nats-io/nats.go"
)

type Config struct {
	ID             string
	Symbol         string
	BuyRatio       float64       // BuyRatio is the fraction of orders that are buys (BID) vs sells (ASK).
	MinQty         float64       // MinQty is the minimum order size.
	MaxQty         float64       // MaxQty is the maximum order size.
	RequestTimeout time.Duration // NOTE FOR MYSELF: if the engine doesnt reply withing X, give up and return a timeout
}

type Trader struct {
	cfg    Config
	nc     *nats.Conn
	ctx    context.Context
	cancel context.CancelFunc
	rng    *rand.Rand

	// Counters for basic benchmarking/visibility.
	submitted     atomic.Uint64
	ackedAccepted atomic.Uint64
	ackedRejected atomic.Uint64
	timeouts      atomic.Uint64
	errors        atomic.Uint64
}

type Stats struct {
	Submitted     uint64 // Submitted counts submit attempts (requests sent).
	AckedAccepted uint64 // AckedAccepted counts acks that indicate the order was accepted.
	AckedRejected uint64 // AckedRejected counts acks that indicate the order was rejected.
	Timeouts      uint64 // Timeouts counts request-reply timeouts.
	Errors        uint64 // Errors counts other submission/encoding/transport errors.
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

	cfg.RequestTimeout = 250 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())

	return &Trader{
		cfg:    cfg,
		nc:     nc,
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

// NOTE TO MYSELF: each trader runs for 100s
func (t *Trader) Run() {
	log.Printf("Trader %s starting: symbol=%s", t.cfg.ID, t.cfg.Symbol)

	ticker := time.NewTicker(100 * time.Second)
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

func (t *Trader) submitOrder() {

	order := t.generateOrder()

	reqData, err := json.Marshal(order)
	if err != nil {
		t.errors.Add(1)
		log.Printf("Error marshaling order: %v", err)
		return
	}

	t.submitted.Add(1)
	msg, err := t.nc.Request(models.SubmitTopic, reqData, t.cfg.RequestTimeout)
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

func (t *Trader) generateOrder() OrderSubmitRequest {
	//
}
