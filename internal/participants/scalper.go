package participants

import (
	"cryptosim/internal/models"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type ScalperMM struct {
	ParticipantConfig ParticipantConfig
	SpreadBps         float64 // target spread in basis points
	OrderSize         float64
	NumLevels         int
	LevelSpacing      float64
	LastQuotedMid     float64
	MinMoveThresh     float64
	BidIDs            []string // slice of orderIDs
	AskIDs            []string // slice of orderIDs
}

func NewScalperMM(participantConfig ParticipantConfig, numLevels int) *ScalperMM {
	return &ScalperMM{
		ParticipantConfig: participantConfig,
		SpreadBps:         2.0,
		OrderSize:         0.01,
		NumLevels:         numLevels, // default 3
		LevelSpacing:      1.0,
		MinMoveThresh:     0.05,
		BidIDs:            make([]string, numLevels),
		AskIDs:            make([]string, numLevels),
	}
}

// -- NATS pubusb --

func (scalperMM *ScalperMM) Start() error {
	_, err := scalperMM.ParticipantConfig.NC.nc.Subscribe(models.PriceBTCTopic, scalperMM.handlePriceInflux)
	if err != nil {
		return fmt.Errorf("subscribe prices.BTC: %w", err)
	}

	_, err3 := scalperMM.ParticipantConfig.NC.nc.Subscribe(string(models.ScalperTradeExecutedTopic), scalperMM.handleTradeReqReply)
	if err3 != nil {
		return fmt.Errorf("subscribe trade executed req reply: %w", err3)
	}

	return nil
}

func (scalperMM *ScalperMM) handlePriceInflux(msg *nats.Msg) {
	var priceTick models.PriceTick

	err := json.Unmarshal(msg.Data, &priceTick)
	if err != nil {
		fmt.Println("ERROR UNMARSHALING JSON IN HANDLEPRICEINFLUX")
		return
	}

	mid := priceTick.Mid
	if abs(mid-scalperMM.LastQuotedMid) < scalperMM.MinMoveThresh {
		return
	}

	scalperMM.cancelAllOrders()
	halfSpread := mid * (scalperMM.SpreadBps / 2) * 0.0001
	spacing := mid * scalperMM.LevelSpacing * 0.0001

	for i := 0; i < scalperMM.NumLevels; i++ {
		offset := halfSpread + float64(i)*spacing
		bidPrice := mid - offset
		askPrice := mid + offset

		scalperMM.BidIDs[i] = scalperMM.submitOrder(models.Bid, models.Limit, bidPrice, scalperMM.OrderSize)
		scalperMM.AskIDs[i] = scalperMM.submitOrder(models.Ask, models.Limit, askPrice, scalperMM.OrderSize)
	}
	scalperMM.LastQuotedMid = mid
}

// -- NATS request-reply --

func (scalperMM *ScalperMM) handleTradeReqReply(msg *nats.Msg) {
	var trade models.Trade
	err := json.Unmarshal(msg.Data, &trade)
	if err != nil {
		scalperMM.replyError(msg, "invalid trade payload")
		return
	}

	ack := models.TradeAck{
		TradeID: trade.TradeID,
	}

	data, _ := json.Marshal(ack)
	msg.Respond(data)
}

func (scalperMM *ScalperMM) replyError(msg *nats.Msg, reason string) {
	ack := models.TradeAck{
		Reason: reason,
	}
	data, _ := json.Marshal(ack)
	msg.Respond(data)
}

func (scalperMM *ScalperMM) submitOrder(side models.Side, orderType models.OrderType, price float64, quantity float64) string {
	order := models.Order{
		ID:         uuid.New().String(),
		Creator_ID: scalperMM.ParticipantConfig.ID,
		Symbol:     scalperMM.ParticipantConfig.Symbol,
		Side:       side,
		OrderType:  orderType,
		Price:      price,
		Qty:        quantity,
		CreatedAt:  time.Now(),
	}

	data, err := json.Marshal(order)
	if err != nil {
		fmt.Println("ERROR MARSHALING ORDER IN submitOrder in SCALPER")
		return ""
	}

	msg, err := scalperMM.ParticipantConfig.NC.nc.Request(models.OrdersSubmitTopic, data, 2*time.Second)
	if err != nil {
		fmt.Println("ERROR SUBMITTING ORDER IN submitOrder in SCALPER")
		return ""
	}

	var ack models.OrderAck
	if err := json.Unmarshal(msg.Data, &ack); err != nil {
		fmt.Println("ERROR UNMARSHALING ACK IN submitOrder in SCALPER")
		return ""
	}

	return ack.OrderID
}

func (scalperMM *ScalperMM) cancelAllOrders() {
	for i, id := range scalperMM.BidIDs {
		scalperMM.cancelOrder(id)
		scalperMM.BidIDs[i] = ""
	}
	for i, id := range scalperMM.AskIDs {
		scalperMM.cancelOrder(id)
		scalperMM.AskIDs[i] = ""
	}
}

func (scalperMM *ScalperMM) cancelOrder(orderID string) {
	if orderID == "" {
		return
	}

	cancelRequest := models.CancelRequest{
		OrderID: orderID,
		Symbol:  scalperMM.ParticipantConfig.Symbol,
	}

	data, err := json.Marshal(cancelRequest)
	if err != nil {
		fmt.Println("ERROR MARSHALING CANCEL REQUEST")
		return
	}

	msg, err := scalperMM.ParticipantConfig.NC.nc.Request(models.OrdersCancelTopic, data, 2*time.Second)
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

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
