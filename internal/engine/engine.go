package engine

import (
	"cryptosim/internal/models"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type Engine struct {
	orderBooks map[string]*OrderBook
	nc         *nats.Conn
}

func NewEngine(nc *nats.Conn) *Engine {
	engine := &Engine{
		orderBooks: make(map[string]*OrderBook),
		nc:         nc,
	}

	engine.orderBooks[string(models.BTC_USD)] = NewOrderBook(string(models.BTC_USD))
	engine.orderBooks[string(models.ETH_USD)] = NewOrderBook(string(models.ETH_USD))
	engine.orderBooks[string(models.XRP_USD)] = NewOrderBook(string(models.XRP_USD))

	return engine
}

func (engine *Engine) Start() error {
	_, err1 := engine.nc.Subscribe(models.OrdersSubmitTopic, engine.handleSubmitOrder)
	if err1 != nil {
		return fmt.Errorf("subscribe submit: %w", err1)
	}

	_, err2 := engine.nc.Subscribe(models.OrdersCancelTopic, engine.handleCancelOrder)
	if err2 != nil {
		return fmt.Errorf("subscribe cancel: %w", err2)
	}

	return nil
}

func (engine *Engine) handleSubmitOrder(msg *nats.Msg) {
	var order models.Order
	err := json.Unmarshal(msg.Data, &order)
	if err != nil {
		engine.replyError(msg, "invalid order payload")
		return
	}

	orderbook, ok := engine.orderBooks[order.Symbol]
	if !ok {
		engine.replyError(msg, "unknown symbol")
		return
	}

	trades := orderbook.SubmitOrder(&order)

	for _, trade := range trades {
		engine.relayTradeExecutionToParticipant(trade)
		engine.publishTrade(trade)
	}

	engine.publishSnapshot(orderbook)

	ack := models.OrderAck{
		OrderID: order.ID,
		Status:  string(order.Status),
	}

	data, _ := json.Marshal(ack)
	msg.Respond(data)
}

func (engine *Engine) relayTradeExecutionToParticipant(trade *models.Trade) {
	data, err := json.Marshal(trade)
	if err != nil {
		fmt.Println("COULD NOT PUBLISH TRADE IN relayTradeExecutionToParticipant")
		return
	}

	var topic models.IndividualTradeTopic
	participant := trade.BuyerID
	switch participant {
	case "avstoikov-mm-1":
		topic = models.AvstoikovTradeExecutedTopic
	case "meanrev-trader-1":
		topic = models.MeanReversionTradeExecutedTopic
	case "momentum-mm-1":
		topic = models.MomentumTradeExecutedTopic
	case "momentum-trader-1":
		topic = models.MomentumChaserTradeExecutedTopic
	case "noise-trader-1":
		topic = models.NoiseTradeExecutedTopic
	case "scalper-mm-1":
		topic = models.ScalperTradeExecutedTopic
	default:
		topic = models.VWAPTradeExecutedTopic
	}

	msg, err := engine.nc.Request(string(topic), data, 2*time.Second)
	if err != nil {
		fmt.Println("ERROR UPDATING PARTICIPANT ABOUT THEIR TRADE IN relayTradeExecutionToParticipant")
		return
	}

	var ack models.TradeAck
	if err := json.Unmarshal(msg.Data, &ack); err != nil {
		fmt.Println("ERROR UNMARSHALING ACK IN updateTradeStatusToParticipant")
		return
	}
}

func (engine *Engine) handleCancelOrder(msg *nats.Msg) {
	var cancelRequest models.CancelRequest
	err := json.Unmarshal(msg.Data, &cancelRequest)
	if err != nil {
		engine.replyError(msg, "invalid cancel payload")
		return

	}

	orderbook, ok := engine.orderBooks[cancelRequest.Symbol]
	if !ok {
		engine.replyError(msg, "unknown symbol")
		return
	}

	success := orderbook.CancelOrder(cancelRequest.OrderID)
	ack := models.CancelAck{
		Success: success,
	}

	data, _ := json.Marshal(ack)
	msg.Respond(data)
}

func (engine *Engine) publishTrade(trade *models.Trade) {
	data, err := json.Marshal(trade)
	if err != nil {
		fmt.Println("COULD NOT PUBLISH TRADE IN PUBLISHTRADE")
		return
	}

	engine.nc.Publish(models.TradesExecutedTopic, data)

}

func (engine *Engine) publishSnapshot(orderbook *OrderBook) {
	asks, bids := orderbook.GetSnapshot(30)
	snapshot := models.OrderbookSnapshot{
		Symbol: orderbook.symbol,
		Bids:   bids,
		Asks:   asks,
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return
	}

	engine.nc.Publish(models.OrderBookSnapshotTopic, data)
}

func (engine *Engine) replyError(msg *nats.Msg, reason string) {
	ack := models.OrderAck{
		Status: "rejected",
		Reason: reason,
	}
	data, _ := json.Marshal(ack)
	msg.Respond(data)
}
