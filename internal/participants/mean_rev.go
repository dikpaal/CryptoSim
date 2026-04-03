package participants

import (
	"cryptosim/internal/models"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type MeanReversionTrader struct {
	ParticipantConfig ParticipantConfig
	Levels            int
	LevelSpacing      float64
	OrderSize         float64
	BaseMid           float64
	RebuildThresh     float64
	BidIDs            []string
	AskIDs            []string
}

func NewMeanReversionTrader(participantConfig ParticipantConfig, numLevels int) *MeanReversionTrader {
	return &MeanReversionTrader{
		ParticipantConfig: participantConfig,
		Levels:            numLevels,
		LevelSpacing:      0.001,
		OrderSize:         0.05,
		BaseMid:           0.0,
		RebuildThresh:     0.005,
		BidIDs:            make([]string, numLevels),
		AskIDs:            make([]string, numLevels),
	}
}

// -- NATS pubusb --

func (meanReversionT *MeanReversionTrader) Start() error {
	_, err := meanReversionT.ParticipantConfig.NC.nc.Subscribe(models.PriceBTCTopic, meanReversionT.handlePriceInflux)
	if err != nil {
		return fmt.Errorf("subscribe prices.ETH: %w", err)
	}
	return nil
}

func (meanReversionT *MeanReversionTrader) handlePriceInflux(msg *nats.Msg) {
	var priceTick models.PriceTick

	err := json.Unmarshal(msg.Data, &priceTick)
	if err != nil {
		fmt.Println("ERROR UNMARSHALING JSON IN HANDLEPRICEINFLUX")
		return
	}

	mid := priceTick.Mid

	if meanReversionT.BaseMid == 0.0 {
		meanReversionT.buildLadders(mid)
		return
	}

	drift := abs(mid-meanReversionT.BaseMid) / meanReversionT.BaseMid
	if drift < meanReversionT.RebuildThresh {
		return // ladder is still valid, wait for fills
	}

	meanReversionT.cancelAllOrders()
	meanReversionT.buildLadders(mid)
}

func (meanReversionT *MeanReversionTrader) buildLadders(mid float64) {
	spacing := mid * meanReversionT.LevelSpacing

	for i := 0; i < meanReversionT.Levels; i++ {
		offset := float64(i+1) * spacing
		bidPrice := mid - offset
		askPrice := mid + offset

		meanReversionT.BidIDs[i] = meanReversionT.submitOrder(models.Bid, models.Limit, bidPrice, meanReversionT.OrderSize)
		meanReversionT.AskIDs[i] = meanReversionT.submitOrder(models.Ask, models.Limit, askPrice, meanReversionT.OrderSize)
	}

	meanReversionT.BaseMid = mid
}

// -- NATS request-reply --

func (meanReversionT *MeanReversionTrader) submitOrder(side models.Side, orderType models.OrderType, price float64, quantity float64) string {
	order := models.Order{
		ID:         uuid.New().String(),
		Creator_ID: meanReversionT.ParticipantConfig.ID,
		Symbol:     meanReversionT.ParticipantConfig.Symbol,
		Side:       side,
		OrderType:  orderType,
		Price:      price,
		Qty:        quantity,
		CreatedAt:  time.Now(),
	}

	data, err := json.Marshal(order)
	if err != nil {
		fmt.Println("ERROR MARSHALING ORDER IN submitOrder in MR_T")
		return ""
	}

	msg, err := meanReversionT.ParticipantConfig.NC.nc.Request(models.OrdersSubmitTopic, data, 2*time.Second)
	if err != nil {
		fmt.Println("ERROR SUBMITTING ORDER IN submitOrder in MR_T")
		return ""
	}

	var ack models.OrderAck
	if err := json.Unmarshal(msg.Data, &ack); err != nil {
		fmt.Println("ERROR UNMARSHALING ACK IN submitOrder in MR_T")
		return ""
	}

	return ack.OrderID
}

func (meanReversionT *MeanReversionTrader) cancelAllOrders() {
	for i, id := range meanReversionT.BidIDs {
		meanReversionT.cancelOrder(id)
		meanReversionT.BidIDs[i] = ""
	}
	for i, id := range meanReversionT.AskIDs {
		meanReversionT.cancelOrder(id)
		meanReversionT.AskIDs[i] = ""
	}
}

func (meanReversionT *MeanReversionTrader) cancelOrder(orderID string) {
	if orderID == "" {
		return
	}

	cancelRequest := models.CancelRequest{
		OrderID: orderID,
		Symbol:  meanReversionT.ParticipantConfig.Symbol,
	}

	data, err := json.Marshal(cancelRequest)
	if err != nil {
		fmt.Println("ERROR MARSHALING CANCEL REQUEST")
		return
	}

	msg, err := meanReversionT.ParticipantConfig.NC.nc.Request(models.OrdersCancelTopic, data, 2*time.Second)
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
