package participants

import (
	"cryptosim/internal/models"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type MomentumMM struct {
	ParticipantConfig ParticipantConfig
	SpreadBps         float64 // target spread in basis points
	OrderSize         float64
	NumLevels         int
	LevelSpacing      float64
	SkewFactor        float64
	PrevMid           float64
	BidIDs            []string // slice of orderIDs
	AskIDs            []string // slice of orderIDs
}

func NewMomentumMM(participantConfig ParticipantConfig, numLevels int) *MomentumMM {
	return &MomentumMM{
		ParticipantConfig: participantConfig,
		SpreadBps:         4.0,
		OrderSize:         0.1,
		NumLevels:         numLevels, // default 5
		LevelSpacing:      2.0,
		SkewFactor:        0.3,
		PrevMid:           -1.0,
		BidIDs:            make([]string, numLevels),
		AskIDs:            make([]string, numLevels),
	}
}

// -- NATS pubusb --

func (momentumMM *MomentumMM) Start() error {
	_, err := momentumMM.ParticipantConfig.NC.nc.Subscribe(models.PriceETHTopic, momentumMM.handlePriceInflux)
	if err != nil {
		return fmt.Errorf("subscribe prices.ETH: %w", err)
	}
	return nil
}

func (momentumMM *MomentumMM) handlePriceInflux(msg *nats.Msg) {
	var priceTick models.PriceTick

	err := json.Unmarshal(msg.Data, &priceTick)
	if err != nil {
		fmt.Println("ERROR UNMARSHALING JSON IN HANDLEPRICEINFLUX")
		return
	}

	momentumMM.cancelAllOrders()

	mid := priceTick.Mid
	halfSpread := mid * (momentumMM.SpreadBps / 2) * 0.0001 // convert to dollars
	spacing := mid * momentumMM.LevelSpacing * 0.0001       // convert spacing to dollars

	var trend float64
	if momentumMM.PrevMid == -1.0 {
		trend = 0
	} else {
		trend = mid - momentumMM.PrevMid
	}

	momentumMM.PrevMid = mid
	skew := trend * momentumMM.SkewFactor

	for i := 0; i < momentumMM.NumLevels; i++ {
		offset := halfSpread + float64(i)*spacing
		bidPrice := mid - offset + skew
		askPrice := mid + offset + skew

		momentumMM.BidIDs[i] = momentumMM.submitOrder(models.Bid, models.Limit, bidPrice, momentumMM.OrderSize)
		momentumMM.AskIDs[i] = momentumMM.submitOrder(models.Ask, models.Limit, askPrice, momentumMM.OrderSize)
	}
}

// -- NATS request-reply --

func (momentumMM *MomentumMM) submitOrder(side models.Side, orderType models.OrderType, price float64, quantity float64) string {
	order := models.Order{
		ID:         uuid.New().String(),
		Creator_ID: momentumMM.ParticipantConfig.ID,
		Symbol:     momentumMM.ParticipantConfig.Symbol,
		Side:       side,
		OrderType:  orderType,
		Price:      price,
		Qty:        quantity,
		CreatedAt:  time.Now(),
	}

	data, err := json.Marshal(order)
	if err != nil {
		fmt.Println("ERROR MARSHALING ORDER IN submitOrder in MOM_MM")
		return ""
	}

	msg, err := momentumMM.ParticipantConfig.NC.nc.Request(models.OrdersSubmitTopic, data, 2*time.Second)
	if err != nil {
		fmt.Println("ERROR SUBMITTING ORDER IN submitOrder in MOM_MM")
		return ""
	}

	var ack models.OrderAck
	if err := json.Unmarshal(msg.Data, &ack); err != nil {
		fmt.Println("ERROR UNMARSHALING ACK IN submitOrder in MOM_MM")
		return ""
	}

	return ack.OrderID
}

func (momentumMM *MomentumMM) cancelAllOrders() {
	for i, id := range momentumMM.BidIDs {
		momentumMM.cancelOrder(id)
		momentumMM.BidIDs[i] = ""
	}
	for i, id := range momentumMM.AskIDs {
		momentumMM.cancelOrder(id)
		momentumMM.AskIDs[i] = ""
	}
}

func (momentumMM *MomentumMM) cancelOrder(orderID string) {
	if orderID == "" {
		return
	}

	cancelRequest := models.CancelRequest{
		OrderID: orderID,
		Symbol:  momentumMM.ParticipantConfig.Symbol,
	}

	data, err := json.Marshal(cancelRequest)
	if err != nil {
		fmt.Println("ERROR MARSHALING CANCEL REQUEST")
		return
	}

	msg, err := momentumMM.ParticipantConfig.NC.nc.Request(models.OrdersCancelTopic, data, 2*time.Second)
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
