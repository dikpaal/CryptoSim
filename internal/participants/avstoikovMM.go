package participants

import (
	"cryptosim/internal/models"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type AvellanedaStoikovMM struct {
	ParticipantConfig ParticipantConfig
	Gamma             float64
	Kappa             float64
	Sigma             float64
	T                 float64
	OrderSize         float64
	NumLevels         int
	LevelSpacing      float64
	BidIDs            []string
	AskIDs            []string
}

func NewAvellanedaStoikovMM(participantConfig ParticipantConfig, numLevels int) *AvellanedaStoikovMM {
	return &AvellanedaStoikovMM{
		ParticipantConfig: participantConfig,
		Gamma:             0.1,
		Kappa:             1.5,
		Sigma:             0.02,
		T:                 1.0,
		OrderSize:         50.0,
		NumLevels:         numLevels, // default 5
		LevelSpacing:      2.0,
		BidIDs:            make([]string, numLevels),
		AskIDs:            make([]string, numLevels),
	}
}

// -- NATS pubusb --

func (avellanedaStoikovMM *AvellanedaStoikovMM) Start() error {
	_, err := avellanedaStoikovMM.ParticipantConfig.NC.nc.Subscribe(models.PriceXRPTopic, avellanedaStoikovMM.handlePriceInflux)
	if err != nil {
		return fmt.Errorf("subscribe prices.XRP: %w", err)
	}

	_, err2 := avellanedaStoikovMM.ParticipantConfig.NC.nc.Subscribe(models.OrderBookSnapshotTopic, avellanedaStoikovMM.handleOrderBookSnapshot)
	if err2 != nil {
		return fmt.Errorf("subscribe orderbook.snapshot: %w", err2)
	}

	return nil
}

func (avellanedaStoikovMM *AvellanedaStoikovMM) handlePriceInflux(msg *nats.Msg) {
	var priceTick models.PriceTick

	err := json.Unmarshal(msg.Data, &priceTick)
	if err != nil {
		fmt.Println("ERROR UNMARSHALING JSON IN HANDLEPRICEINFLUX")
		return
	}

	avellanedaStoikovMM.cancelAllOrders()

	mid := priceTick.Mid
	gamma := avellanedaStoikovMM.Gamma
	position := avellanedaStoikovMM.ParticipantConfig.Position
	sigma := avellanedaStoikovMM.Sigma
	kappa := avellanedaStoikovMM.Kappa

	reservationPrice := mid - position*gamma*math.Pow(sigma, 2)*avellanedaStoikovMM.T
	spread := gamma*math.Pow(sigma, 2) + (2/gamma)*math.Log(1+gamma/kappa)
	spacing := mid * avellanedaStoikovMM.LevelSpacing * 0.0001 // convert spacing to dollars

	for i := 0; i < avellanedaStoikovMM.NumLevels; i++ {
		bidPrice := reservationPrice - spread/2 - float64(i)*spacing
		askPrice := reservationPrice + spread/2 + float64(i)*spacing

		avellanedaStoikovMM.BidIDs[i] = avellanedaStoikovMM.submitOrder(models.Bid, models.Limit, bidPrice, avellanedaStoikovMM.OrderSize)
		avellanedaStoikovMM.AskIDs[i] = avellanedaStoikovMM.submitOrder(models.Ask, models.Limit, askPrice, avellanedaStoikovMM.OrderSize)
	}
}

func (avellanedaStoikovMM *AvellanedaStoikovMM) handleOrderBookSnapshot(msg *nats.Msg) {
	var snapshot models.OrderbookSnapshot
	if err := json.Unmarshal(msg.Data, &snapshot); err != nil {
		fmt.Println("ERROR UNMARSHALING ORDERBOOK SNAPSHOT")
		return
	}

	if snapshot.Symbol != avellanedaStoikovMM.ParticipantConfig.Symbol {
		return
	}

	if len(snapshot.Bids) == 0 || len(snapshot.Asks) == 0 {
		return
	}

	bestBidVol := snapshot.Bids[0][1] // [price, quantity]
	bestAskVol := snapshot.Asks[0][1]

	avgVol := (bestBidVol + bestAskVol) / 2
	avellanedaStoikovMM.Kappa = math.Log(1 + avgVol)

}

// -- NATS request-reply --

func (avellanedaStoikovMM *AvellanedaStoikovMM) submitOrder(side models.Side, orderType models.OrderType, price float64, quantity float64) string {
	order := models.Order{
		ID:         uuid.New().String(),
		Creator_ID: avellanedaStoikovMM.ParticipantConfig.ID,
		Symbol:     avellanedaStoikovMM.ParticipantConfig.Symbol,
		Side:       side,
		OrderType:  orderType,
		Price:      price,
		Qty:        quantity,
		CreatedAt:  time.Now(),
	}

	data, err := json.Marshal(order)
	if err != nil {
		fmt.Println("ERROR MARSHALING ORDER IN submitOrder in AV_MM")
		return ""
	}

	msg, err := avellanedaStoikovMM.ParticipantConfig.NC.nc.Request(models.OrdersSubmitTopic, data, 2*time.Second)
	if err != nil {
		fmt.Println("ERROR SUBMITTING ORDER IN submitOrder in AV_MM")
		return ""
	}

	var ack models.OrderAck
	if err := json.Unmarshal(msg.Data, &ack); err != nil {
		fmt.Println("ERROR UNMARSHALING ACK IN submitOrder in AV_MM")
		return ""
	}

	return ack.OrderID
}

func (avellanedaStoikovMM *AvellanedaStoikovMM) cancelAllOrders() {
	for i, id := range avellanedaStoikovMM.BidIDs {
		avellanedaStoikovMM.cancelOrder(id)
		avellanedaStoikovMM.BidIDs[i] = ""
	}
	for i, id := range avellanedaStoikovMM.AskIDs {
		avellanedaStoikovMM.cancelOrder(id)
		avellanedaStoikovMM.AskIDs[i] = ""
	}
}

func (avellanedaStoikovMM *AvellanedaStoikovMM) cancelOrder(orderID string) {
	if orderID == "" {
		return
	}

	cancelRequest := models.CancelRequest{
		OrderID: orderID,
		Symbol:  avellanedaStoikovMM.ParticipantConfig.Symbol,
	}

	data, err := json.Marshal(cancelRequest)
	if err != nil {
		fmt.Println("ERROR MARSHALING CANCEL REQUEST")
		return
	}

	msg, err := avellanedaStoikovMM.ParticipantConfig.NC.nc.Request(models.OrdersCancelTopic, data, 2*time.Second)
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
