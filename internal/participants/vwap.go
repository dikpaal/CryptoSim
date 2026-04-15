package participants

import (
	"cryptosim/internal/models"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type VWAPTrader struct {
	ParticipantConfig ParticipantConfig
	Window            int
	Trades            []models.Trade
	VWAP              float64
	Threshold         float64
	OrderSize         float64
	ActiveOrderID     string
	LatestMid         float64
	mu                sync.Mutex
}

func NewVWAPTrader(participantConfig ParticipantConfig) *VWAPTrader {
	return &VWAPTrader{
		ParticipantConfig: participantConfig,
		Window:            50,
		Trades:            make([]models.Trade, 0, 50),
		VWAP:              0.0,
		Threshold:         0.001,
		OrderSize:         0.05,
		ActiveOrderID:     "",
		LatestMid:         0.0,
	}
}

func (t *VWAPTrader) Start() error {
	_, err1 := t.ParticipantConfig.NC.nc.Subscribe(models.PriceBTCTopic, t.handlePriceInflux)
	if err1 != nil {
		return fmt.Errorf("subscribe prices.BTC: %w", err1)
	}
	_, err2 := t.ParticipantConfig.NC.nc.Subscribe(models.TradesExecutedTopic, t.handleTradeExecuted)
	if err2 != nil {
		return fmt.Errorf("subscribe trades.executed: %w", err2)
	}

	_, err3 := t.ParticipantConfig.NC.nc.Subscribe(string(models.VWAPTradeExecutedTopic), t.handleTradeReqReply)
	if err3 != nil {
		return fmt.Errorf("subscribe trade executed req reply: %w", err3)
	}

	return nil
}

func (t *VWAPTrader) handleTradeExecuted(msg *nats.Msg) {
	var trade models.Trade
	if err := json.Unmarshal(msg.Data, &trade); err != nil {
		fmt.Println("ERROR UNMARSHALING TRADE IN VWAP")
		return
	}

	// only care about our symbol
	if trade.Symbol != t.ParticipantConfig.Symbol {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// maintain rolling window
	t.Trades = append(t.Trades, trade)
	if len(t.Trades) > t.Window {
		t.Trades = t.Trades[1:]
	}

	// recompute VWAP
	var totalVolume, totalValue float64
	for _, tr := range t.Trades {
		totalVolume += tr.Qty
		totalValue += tr.Price * tr.Qty
	}
	if totalVolume > 0 {
		t.VWAP = totalValue / totalVolume
	}
}

func (t *VWAPTrader) handlePriceInflux(msg *nats.Msg) {
	var priceTick models.PriceTick
	if err := json.Unmarshal(msg.Data, &priceTick); err != nil {
		fmt.Println("ERROR UNMARSHALING JSON IN HANDLEPRICEINFLUX")
		return
	}

	t.mu.Lock()
	mid := priceTick.Mid
	t.LatestMid = mid
	vwap := t.VWAP
	activeOrderID := t.ActiveOrderID
	t.ActiveOrderID = ""
	t.mu.Unlock()

	if vwap == 0.0 {
		return
	}

	t.cancelOrder(activeOrderID)

	deviation := (mid - vwap) / vwap

	var newOrderID string
	if deviation < -t.Threshold {
		newOrderID = t.submitOrder(models.Bid, models.Limit, mid, t.OrderSize)
	} else if deviation > t.Threshold {
		newOrderID = t.submitOrder(models.Ask, models.Limit, mid, t.OrderSize)
	}

	t.mu.Lock()
	t.ActiveOrderID = newOrderID
	t.mu.Unlock()
}

func (t *VWAPTrader) handleTradeReqReply(msg *nats.Msg) {
	var trade models.Trade
	err := json.Unmarshal(msg.Data, &trade)
	if err != nil {
		t.replyError(msg, "invalid trade payload")
		return
	}

	ack := models.TradeAck{
		TradeID: trade.TradeID,
	}

	data, _ := json.Marshal(ack)
	msg.Respond(data)
}

func (t *VWAPTrader) replyError(msg *nats.Msg, reason string) {
	ack := models.TradeAck{
		Reason: reason,
	}
	data, _ := json.Marshal(ack)
	msg.Respond(data)
}

func (t *VWAPTrader) submitOrder(side models.Side, orderType models.OrderType, price float64, quantity float64) string {
	order := models.Order{
		ID:         uuid.New().String(),
		Creator_ID: t.ParticipantConfig.ID,
		Symbol:     t.ParticipantConfig.Symbol,
		Side:       side,
		OrderType:  orderType,
		Price:      price,
		Qty:        quantity,
		CreatedAt:  time.Now(),
	}

	data, err := json.Marshal(order)
	if err != nil {
		fmt.Println("ERROR MARSHALING ORDER IN submitOrder in VWAP_T")
		return ""
	}

	msg, err := t.ParticipantConfig.NC.nc.Request(models.OrdersSubmitTopic, data, 2*time.Second)
	if err != nil {
		fmt.Println("ERROR SUBMITTING ORDER IN submitOrder in VWAP_T")
		return ""
	}

	var ack models.OrderAck
	if err := json.Unmarshal(msg.Data, &ack); err != nil {
		fmt.Println("ERROR UNMARSHALING ACK IN submitOrder in VWAP_T")
		return ""
	}

	return ack.OrderID
}

func (t *VWAPTrader) cancelOrder(orderID string) {
	if orderID == "" {
		return
	}

	cancelRequest := models.CancelRequest{
		OrderID: orderID,
		Symbol:  t.ParticipantConfig.Symbol,
	}

	data, err := json.Marshal(cancelRequest)
	if err != nil {
		fmt.Println("ERROR MARSHALING CANCEL REQUEST")
		return
	}

	msg, err := t.ParticipantConfig.NC.nc.Request(models.OrdersCancelTopic, data, 2*time.Second)
	if err != nil {
		fmt.Println("ERROR SENDING CANCEL REQUEST")
		return
	}

	var ack models.CancelAck
	if err := json.Unmarshal(msg.Data, &ack); err != nil {
		fmt.Println("ERROR UNMARSHALING CANCEL ACK")
		return
	}
}
