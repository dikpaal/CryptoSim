package trader

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
	ID           string
	Symbol       string
	MaxInventory float64
	MaxOrders    int
	Strategy     Strategy
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

type Trader struct {
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

func NewTrader(nc *nats.Conn, cfg Config) *Trader {
	ctx, cancel := context.WithCancel(context.Background())
	return &Trader{
		cfg:          cfg,
		nc:           nc,
		strategy:     cfg.Strategy,
		activeOrders: make(map[string]bool),
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (trader *Trader) Run() error {
	if _, err := trader.nc.Subscribe(models.PricesLiveTopic, trader.handlePriceTick); err != nil {
		return err
	}
	log.Printf("trader %s subscribed to %s", trader.cfg.ID, models.PricesLiveTopic)

	if _, err := trader.nc.Subscribe(models.TradesExecutedTopic, trader.handleTradeExecuted); err != nil {
		return err
	}
	log.Printf("trader %s subscribed to %s", trader.cfg.ID, models.TradesExecutedTopic)

	go trader.publishStatusLoop()

	log.Printf("trader %s (%s) started", trader.cfg.ID, trader.strategy.Name())
	<-trader.ctx.Done()
	return nil
}

func (trader *Trader) Stop() {
	trader.cancel()
}

func (trader *Trader) handlePriceTick(msg *nats.Msg) {
	var tick PriceTick
	if err := json.Unmarshal(msg.Data, &tick); err != nil {
		log.Printf("Error unmarshaling price tick: %v", err)
		return
	}

	trader.mu.Lock()
	trader.currentMid = tick.Mid
	inventory := trader.inventory
	trader.mu.Unlock()

	quote := trader.strategy.OnPriceTick(tick, inventory)
	if quote == nil {
		return
	}

	trader.mu.RLock()
	openOrders := len(trader.activeOrders)
	trader.mu.RUnlock()

	if abs(inventory) >= trader.cfg.MaxInventory {
		log.Printf("trader %s: inventory limit reached (%f)", trader.cfg.ID, inventory)
		return
	}

	if openOrders >= trader.cfg.MaxOrders {
		trader.cancelAllOrders()
	}

	trader.submitQuote(quote)
}

func (trader *Trader) handleTradeExecuted(msg *nats.Msg) {
	var trade models.Trade
	if err := json.Unmarshal(msg.Data, &trade); err != nil {
		log.Printf("Error unmarshaling trade: %v", err)
		return
	}

	trader.mu.Lock()
	defer trader.mu.Unlock()

	isBuyer := trade.BuyerID == trader.cfg.ID
	isSeller := trade.SellerID == trader.cfg.ID

	if !isBuyer && !isSeller {
		return
	}

	delete(trader.activeOrders, trade.BuyerOrderID)
	delete(trader.activeOrders, trade.SellerOrderID)

	oldInventory := trader.inventory
	if isBuyer {
		trader.inventory += trade.Qty
	}
	if isSeller {
		trader.inventory -= trade.Qty
	}

	if oldInventory != 0 && signChanged(oldInventory, trader.inventory) {
		trader.realizedPnL += oldInventory * (trade.Price - trader.avgCost)
		trader.avgCost = trade.Price
	} else {
		totalCost := trader.avgCost*abs(oldInventory) + trade.Price*trade.Qty
		trader.avgCost = totalCost / abs(trader.inventory)
	}
}

func (trader *Trader) submitQuote(quote *Quote) {
	if quote.BidQty > 0 && quote.BidPrice > 0 {
		trader.submitOrder("BID", "LIMIT", quote.BidPrice, quote.BidQty)
	}
	if quote.AskQty > 0 && quote.AskPrice > 0 {
		trader.submitOrder("ASK", "LIMIT", quote.AskPrice, quote.AskQty)
	}
}

func (trader *Trader) submitOrder(side, orderType string, price, qty float64) {
	req := map[string]interface{}{
		"client_order_id": uuid.New().String(),
		"id":              trader.cfg.ID,
		"symbol":          trader.cfg.Symbol,
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

	msg, err := trader.nc.Request(models.OrdersSubmitTopic, reqData, 250*time.Millisecond)
	if err != nil {
		return
	}

	var reply map[string]interface{}
	if err := json.Unmarshal(msg.Data, &reply); err != nil {
		return
	}

	if accepted, ok := reply["accepted"].(bool); ok && accepted {
		if orderID, ok := reply["order_id"].(string); ok {
			trader.mu.Lock()
			trader.activeOrders[orderID] = true
			trader.mu.Unlock()
		}
	}
}

func (trader *Trader) cancelAllOrders() {
	trader.mu.Lock()
	orderIDs := make([]string, 0, len(trader.activeOrders))
	for orderID := range trader.activeOrders {
		orderIDs = append(orderIDs, orderID)
	}
	trader.mu.Unlock()

	for _, orderID := range orderIDs {
		trader.cancelOrder(orderID)
	}
}

func (trader *Trader) cancelOrder(orderID string) {
	req := map[string]interface{}{
		"client_cancel_id": uuid.New().String(),
		"id":               trader.cfg.ID,
		"order_id":         orderID,
		"timestamp":        time.Now().UnixNano(),
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return
	}

	trader.nc.Request(models.OrdersCancelTopic, reqData, 250*time.Millisecond)

	trader.mu.Lock()
	delete(trader.activeOrders, orderID)
	trader.mu.Unlock()
}

func (trader *Trader) publishStatusLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-trader.ctx.Done():
			return
		case <-ticker.C:
			trader.publishStatus()
		}
	}
}

func (trader *Trader) publishStatus() {
	trader.mu.RLock()
	inventory := trader.inventory
	realizedPnL := trader.realizedPnL
	currentMid := trader.currentMid
	openOrders := len(trader.activeOrders)
	trader.mu.RUnlock()

	unrealizedPnL := 0.0
	if inventory != 0 && currentMid > 0 {
		unrealizedPnL = inventory * (currentMid - trader.avgCost)
	}

	status := Status{
		ID:            trader.cfg.ID,
		Strategy:      trader.strategy.Name(),
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

	trader.nc.Publish(models.StatusTopic, data)
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
