package mm

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

type MMStatus struct {
	MMID          string  `json:"mm_id"`
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
		cfg:          cfg,
		nc:           nc,
		strategy:     cfg.Strategy,
		activeOrders: make(map[string]bool),
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (marketMaker *MarketMaker) Run() error {
	if _, err := marketMaker.nc.Subscribe(models.PricesLiveTopic, marketMaker.handlePriceTick); err != nil {
		return err
	}
	log.Printf("marketMaker %s subscribed to %s", marketMaker.cfg.ID, models.PricesLiveTopic)

	if _, err := marketMaker.nc.Subscribe(models.TradesExecutedTopic, marketMaker.handleTradeExecuted); err != nil {
		return err
	}
	log.Printf("marketMaker %s subscribed to %s", marketMaker.cfg.ID, models.TradesExecutedTopic)

	go marketMaker.publishStatusLoop()

	log.Printf("marketMaker %s (%s) started", marketMaker.cfg.ID, marketMaker.strategy.Name())
	<-marketMaker.ctx.Done()
	return nil
}

func (marketMaker *MarketMaker) Stop() {
	marketMaker.cancel()
}

func (marketMaker *MarketMaker) handlePriceTick(msg *nats.Msg) {
	var tick PriceTick
	err := json.Unmarshal(msg.Data, &tick)
	if err != nil {
		log.Printf("Error unmarshaling price tick: %v", err)
		return
	}

	marketMaker.mu.Lock()
	marketMaker.currentMid = tick.Mid
	inventory := marketMaker.inventory
	marketMaker.mu.Unlock()

	quote := marketMaker.strategy.OnPriceTick(tick, inventory)
	if quote == nil {
		return
	}

	marketMaker.mu.RLock()
	openOrders := len(marketMaker.activeOrders)
	marketMaker.mu.RUnlock()

	if abs(inventory) >= marketMaker.cfg.MaxInventory {
		log.Printf("marketMaker %s: inventory limit reached (%f)", marketMaker.cfg.ID, inventory)
		return
	}

	if openOrders >= marketMaker.cfg.MaxOrders {
		marketMaker.cancelAllOrders()
	}

	marketMaker.submitQuote(quote)
}

func (marketMaker *MarketMaker) handleTradeExecuted(msg *nats.Msg) {
	var trade models.Trade
	err := json.Unmarshal(msg.Data, &trade)
	if err != nil {
		log.Printf("Error unmarshaling trade: %v", err)
		return
	}

	marketMaker.mu.Lock()
	defer marketMaker.mu.Unlock()

	isBuyer := trade.BuyerMMID == marketMaker.cfg.ID
	isSeller := trade.SellerMMID == marketMaker.cfg.ID

	if !isBuyer && !isSeller {
		return
	}

	delete(marketMaker.activeOrders, trade.BuyerOrderID)
	delete(marketMaker.activeOrders, trade.SellerOrderID)

	oldInventory := marketMaker.inventory
	if isBuyer {
		marketMaker.inventory += trade.Qty
	}
	if isSeller {
		marketMaker.inventory -= trade.Qty
	}

	if oldInventory > 0 && signChanged(oldInventory, marketMaker.inventory) {
		marketMaker.realizedPnL += oldInventory * (trade.Price - marketMaker.avgCost)
	} else {
		totalCost := marketMaker.avgCost*abs(oldInventory) + trade.Price*trade.Qty
		marketMaker.avgCost = totalCost / abs(marketMaker.inventory)
	}
}

func (marketMaker *MarketMaker) submitQuote(quote *Quote) {
	if quote.BidQty > 0 && quote.BidPrice > 0 {
		marketMaker.submitOrder("BID", "LIMIT", quote.BidPrice, quote.BidQty)
	}
	if quote.AskQty > 0 && quote.AskPrice > 0 {
		marketMaker.submitOrder("ASK", "LIMIT", quote.AskPrice, quote.AskQty)
	}
}

func (marketMaker *MarketMaker) submitOrder(side, orderType string, price, qty float64) {
	req := map[string]interface{}{
		"client_order_id": uuid.New().String(),
		"marketMaker_id":  marketMaker.cfg.ID,
		"symbol":          marketMaker.cfg.Symbol,
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

	msg, err := marketMaker.nc.Request(models.OrdersSubmitTopic, reqData, 250*time.Millisecond)
	if err != nil {
		return
	}

	var reply map[string]interface{}
	if err := json.Unmarshal(msg.Data, &reply); err != nil {
		return
	}

	accepted, ok := reply["accepted"].(bool)
	if accepted && ok {
		orderID, ok := reply["order_id"].(string)
		if ok {
			marketMaker.mu.Lock()
			marketMaker.activeOrders[orderID] = true
			marketMaker.mu.Unlock()
		}
	}
}

func (marketMaker *MarketMaker) cancelAllOrders() {
	marketMaker.mu.Lock()
	orderIDs := make([]string, 0, len(marketMaker.activeOrders))
	for orderID := range marketMaker.activeOrders {
		orderIDs = append(orderIDs, orderID)
	}
	marketMaker.mu.Unlock()

	for _, orderID := range orderIDs {
		marketMaker.cancelOrder(orderID)
	}
}

func (marketMaker *MarketMaker) cancelOrder(orderID string) {
	req := map[string]interface{}{
		"client_cancel_id": uuid.New().String(),
		"marketMaker_id":   marketMaker.cfg.ID,
		"order_id":         orderID,
		"timestamp":        time.Now().UnixNano(),
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return
	}

	marketMaker.nc.Request(models.OrdersCancelTopic, reqData, 250*time.Millisecond)

	marketMaker.mu.Lock()
	delete(marketMaker.activeOrders, orderID)
	marketMaker.mu.Unlock()
}

func (marketMaker *MarketMaker) publishStatusLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-marketMaker.ctx.Done():
			return
		case <-ticker.C:
			marketMaker.publishStatus()
		}
	}
}

func (marketMaker *MarketMaker) publishStatus() {
	marketMaker.mu.RLock()
	inventory := marketMaker.inventory
	realizedPnL := marketMaker.realizedPnL
	currentMid := marketMaker.currentMid
	openOrders := len(marketMaker.activeOrders)
	marketMaker.mu.RUnlock()

	unrealizedPnL := 0.0
	if inventory != 0 && currentMid > 0 {
		unrealizedPnL = inventory * (currentMid - marketMaker.avgCost)
	}

	status := MMStatus{
		MMID:          marketMaker.cfg.ID,
		Strategy:      marketMaker.strategy.Name(),
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

	marketMaker.nc.Publish(models.MMStatusTopic, data)
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
