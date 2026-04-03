package participants

import (
	"cryptosim/internal/models"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"math/rand"
	"sync"
	"time"
)

type NoiseTrader struct {
	ParticipantConfig ParticipantConfig
	MinInterval       time.Duration
	MaxInterval       time.Duration
	MinSize           float64
	MaxSize           float64
	LatestMid         float64
	mu                sync.Mutex
}

func NewNoiseTrader(participantConfig ParticipantConfig) *NoiseTrader {
	return &NoiseTrader{
		ParticipantConfig: participantConfig,
		MinInterval:       50 * time.Millisecond,
		MaxInterval:       500 * time.Millisecond,
		MinSize:           0.001,
		MaxSize:           0.05,
	}
}

// -- NATS pubusb --

func (noiseT *NoiseTrader) Start() error {
	_, err := noiseT.ParticipantConfig.NC.nc.Subscribe(models.PriceXRPTopic, noiseT.handlePriceInflux)
	if err != nil {
		return fmt.Errorf("subscribe prices.XRP: %w", err)
	}
	go noiseT.run()
	return nil
}

func (noiseT *NoiseTrader) handlePriceInflux(msg *nats.Msg) {
	var priceTick models.PriceTick
	if err := json.Unmarshal(msg.Data, &priceTick); err != nil {
		fmt.Println("ERROR UNMARSHALING JSON IN HANDLEPRICEINFLUX")
		return
	}

	noiseT.mu.Lock()
	noiseT.LatestMid = priceTick.Mid
	noiseT.mu.Unlock()
}

func (noiseT *NoiseTrader) run() {

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	for {
		noiseT.mu.Lock()
		mid := noiseT.LatestMid
		noiseT.mu.Unlock()

		if mid == 0.0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// random side
		side := models.Bid
		if r.Intn(2) == 1 {
			side = models.Ask
		}

		// random order type
		orderType := models.Limit
		if r.Intn(2) == 1 {
			orderType = models.Market
		}

		// random size
		size := noiseT.MinSize + r.Float64()*(noiseT.MaxSize-noiseT.MinSize)

		// random price offset (only matters for limit orders)
		maxOffset := noiseT.LatestMid * 0.001
		offset := r.Float64() * maxOffset
		var price float64
		if side == models.Bid {
			price = noiseT.LatestMid - offset
		} else {
			price = noiseT.LatestMid + offset
		}

		noiseT.submitOrder(side, orderType, price, size)

		// random sleep
		interval := noiseT.MinInterval + time.Duration(r.Int63n(int64(noiseT.MaxInterval-noiseT.MinInterval)))
		time.Sleep(interval)
	}
}

// -- NATS request-reply --

func (noiseT *NoiseTrader) submitOrder(side models.Side, orderType models.OrderType, price float64, quantity float64) string {
	order := models.Order{
		ID:         uuid.New().String(),
		Creator_ID: noiseT.ParticipantConfig.ID,
		Symbol:     noiseT.ParticipantConfig.Symbol,
		Side:       side,
		OrderType:  orderType,
		Price:      price,
		Qty:        quantity,
		CreatedAt:  time.Now(),
	}

	data, err := json.Marshal(order)
	if err != nil {
		fmt.Println("ERROR MARSHALING ORDER IN submitOrder in NOISE_T")
		return ""
	}

	msg, err := noiseT.ParticipantConfig.NC.nc.Request(models.OrdersSubmitTopic, data, 2*time.Second)
	if err != nil {
		fmt.Println("ERROR SUBMITTING ORDER IN submitOrder in NOISE_T")
		return ""
	}

	var ack models.OrderAck
	if err := json.Unmarshal(msg.Data, &ack); err != nil {
		fmt.Println("ERROR UNMARSHALING ACK IN submitOrder in NOISE_T")
		return ""
	}

	return ack.OrderID
}
