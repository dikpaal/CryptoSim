package marketmaker

import (
	"context"
	"cryptosim/internal/models"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type Config struct {
	ID                  string
	Symbol              string
	MaxInventory        float64
	MaxOrders           int
	Strategy            Strategy
	TradesExecutedTopic string
}

type Status struct {
	ID            string  `json:"id"`
	Strategy      string  `json:"strategy"`
	Inventory     float64 `json:"inventory"`
	RealizedPnL   float64 `json:"realized_pnl"`
	UnrealizedPnL float64 `json:"unrealized_pnl"`
	OpenOrders    int     `json:"open_orders"`
	Timestamp     int64   `json:"timestamp"`
}

type MarketMaker struct {
	cfg      Config
	nc       *nats.Conn
	strategy Strategy

	mu           sync.RWMutex
	inventory    float64
	realizedPnL  float64
	avgCost      float64
	currentMid   float64
	activeOrders map[string]bool

	ctx    context.Context
	cancel context.CancelFunc
}

func NewMarketMaker(nc *nats.Conn, cfg Config) *MarketMaker {
	ctx, cancel := context.WithCancel(context.Background())
	return &MarketMaker{
		cfg:      cfg,
		nc:       nc,
		strategy: cfg.Strategy,

		activeOrders: make(map[string]bool),
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (mm *MarketMaker) Run() error {
	if _, err := mm.nc.Subscribe(models.PricesLiveTopic, mm.handlePriceTick); err != nil {
		return err
	}
	log.Printf("MM %s subscribed to %s", mm.cfg.ID, models.PricesLiveTopic)

	if _, err := mm.nc.Subscribe(mm.cfg.TradesExecutedTopic, mm.handleTradeExecuted); err != nil {
		return err
	}
	log.Printf("MM %s subscribed to %s", mm.cfg.ID, mm.cfg.TradesExecutedTopic)

	go mm.publishStatusLoop()

	log.Printf("MM %s (%s) started", mm.cfg.ID, mm.strategy.Name())
	<-mm.ctx.Done()
	return nil
}

func (mm *MarketMaker) Stop() {
	mm.cancel()
}

func (mm *MarketMaker) handlePriceTick(msg *nats.Msg) {
	var tick PriceTick
	if err := json.Unmarshal(msg.Data, &tick); err != nil {
		log.Printf("Error unmarshaling price tick: %v", err)
		return
	}

	mm.mu.Lock()
	mm.currentMid = tick.Mid
	inventory := mm.inventory
	mm.mu.Unlock()

	quote := mm.strategy.OnPriceTick(tick, inventory)
	if quote == nil {
		return
	}

	mm.mu.RLock()
	openOrders := len(mm.activeOrders)
	mm.mu.RUnlock()

	if abs(inventory) >= mm.cfg.MaxInventory {
		log.Printf("MM %s: inventory limit reached (%f)", mm.cfg.ID, inventory)
		return
	}

	if openOrders >= mm.cfg.MaxOrders {
		mm.cancelAllOrders()
	}

	mm.submitQuote(quote)
}

func (mm *MarketMaker) handleTradeExecuted(msg *nats.Msg) {
	var trade models.Trade
	if err := json.Unmarshal(msg.Data, &trade); err != nil {
		log.Printf("Error unmarshaling trade: %v", err)
		return
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()

	isBuyer := trade.BuyerID == mm.cfg.ID
	isSeller := trade.SellerID == mm.cfg.ID

	if !isBuyer && !isSeller {
		return
	}

	delete(mm.activeOrders, trade.BuyerOrderID)
	delete(mm.activeOrders, trade.SellerOrderID)

	oldInventory := mm.inventory
	if isBuyer {
		mm.inventory += trade.Qty
	}
	if isSeller {
		mm.inventory -= trade.Qty
	}

	if oldInventory != 0 && signChanged(oldInventory, mm.inventory) {
		mm.realizedPnL += oldInventory * (trade.Price - mm.avgCost)
		mm.avgCost = trade.Price
	} else {
		totalCost := mm.avgCost*abs(oldInventory) + trade.Price*trade.Qty
		mm.avgCost = totalCost / abs(mm.inventory)
	}
}

func (mm *MarketMaker) submitQuote(quote *Quote) {
	if quote.BidQty > 0 && quote.BidPrice > 0 {
		mm.submitOrder("BID", "LIMIT", quote.BidPrice, quote.BidQty)
	}
	if quote.AskQty > 0 && quote.AskPrice > 0 {
		mm.submitOrder("ASK", "LIMIT", quote.AskPrice, quote.AskQty)
	}
}

func (mm *MarketMaker) submitOrder(side, orderType string, price, qty float64) {
	req := map[string]interface{}{
		"client_order_id": uuid.New().String(),
		"id":              mm.cfg.ID,
		"symbol":          mm.cfg.Symbol,
		"side":            side,
		"type":            orderType,
		"price":           price,
		"qty":             qty,
		"timestamp":       time.Now().UnixNano(),
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		log.Printf("Error marshaling order: %v", err)
		return
	}

	msg, err := mm.nc.Request(models.OrdersSubmitTopic, reqData, 250*time.Millisecond)
	if err != nil {
		return
	}

	var reply map[string]interface{}
	if err := json.Unmarshal(msg.Data, &reply); err != nil {
		return
	}

	if accepted, ok := reply["accepted"].(bool); ok && accepted {
		if orderID, ok := reply["order_id"].(string); ok {
			mm.mu.Lock()
			mm.activeOrders[orderID] = true
			mm.mu.Unlock()
		}
	}
}

func (mm *MarketMaker) cancelAllOrders() {
	mm.mu.Lock()
	orderIDs := make([]string, 0, len(mm.activeOrders))
	for orderID := range mm.activeOrders {
		orderIDs = append(orderIDs, orderID)
	}
	mm.mu.Unlock()

	for _, orderID := range orderIDs {
		mm.cancelOrder(orderID)
	}
}

func (mm *MarketMaker) cancelOrder(orderID string) {
	req := map[string]interface{}{
		"client_cancel_id": uuid.New().String(),
		"creator_id":       mm.cfg.ID,
		"order_id":         orderID,
		"timestamp":        time.Now().UnixNano(),
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return
	}

	mm.nc.Request(models.OrdersCancelTopic, reqData, 250*time.Millisecond)

	mm.mu.Lock()
	delete(mm.activeOrders, orderID)
	mm.mu.Unlock()
}

func (mm *MarketMaker) publishStatusLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-mm.ctx.Done():
			return
		case <-ticker.C:
			mm.publishStatus()
		}
	}
}

func (mm *MarketMaker) publishStatus() {
	mm.mu.RLock()
	inventory := mm.inventory
	realizedPnL := mm.realizedPnL
	currentMid := mm.currentMid
	openOrders := len(mm.activeOrders)
	mm.mu.RUnlock()

	unrealizedPnL := 0.0
	if inventory != 0 && currentMid > 0 {
		unrealizedPnL = inventory * (currentMid - mm.avgCost)
	}

	status := Status{
		ID:            mm.cfg.ID,
		Strategy:      mm.strategy.Name(),
		Inventory:     inventory,
		RealizedPnL:   realizedPnL,
		UnrealizedPnL: unrealizedPnL,
		OpenOrders:    openOrders,
		Timestamp:     time.Now().UnixNano(),
	}

	data, err := json.Marshal(status)
	if err != nil {
		return
	}

	mm.nc.Publish(models.StatusTopic, data)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func signChanged(old, new float64) bool {
	return (old > 0 && new < 0) || (old < 0 && new > 0)
}
