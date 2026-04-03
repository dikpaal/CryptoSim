package participants

import (
	"cryptosim/internal/models"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type MomentumT struct {
	ParticipantConfig ParticipantConfig
	WindowSize        int
	PriceWindow       []float64
	OrderSize         float64
	Threshold         float64
	ActiveOrderID     string
	ProfitOrderID     string
}

func NewMomentumT(participantConfig ParticipantConfig, windowSize int) *MomentumT {
	return &MomentumT{
		ParticipantConfig: participantConfig,
		WindowSize:        windowSize,
		PriceWindow:       make([]float64, 0, windowSize),
		OrderSize:         0.05,
		Threshold:         0.002, // 0.2% price move to trigger
		ActiveOrderID:     "",
		ProfitOrderID:     "",
	}
}

// -- NATS pubusb --

func (momentumT *MomentumT) Start() error {
	_, err := momentumT.ParticipantConfig.NC.nc.Subscribe(models.PriceETHTopic, momentumT.handlePriceInflux)
	if err != nil {
		return fmt.Errorf("subscribe prices.ETH: %w", err)
	}
	return nil
}

func (momentumT *MomentumT) handlePriceInflux(msg *nats.Msg) {
	var priceTick models.PriceTick
	if err := json.Unmarshal(msg.Data, &priceTick); err != nil {
		fmt.Println("ERROR UNMARSHALING JSON IN HANDLEPRICEINFLUX")
		return
	}

	momentumT.PriceWindow = append(momentumT.PriceWindow, priceTick.Price)
	if len(momentumT.PriceWindow) > momentumT.WindowSize {
		momentumT.PriceWindow = momentumT.PriceWindow[1:]
	}

	if len(momentumT.PriceWindow) < momentumT.WindowSize {
		return // not enough data
	}

	trend := momentumT.PriceWindow[len(momentumT.PriceWindow)-1] - momentumT.PriceWindow[0]

	momentumT.cancelOrder(momentumT.ActiveOrderID)
	momentumT.ActiveOrderID = ""
	momentumT.cancelOrder(momentumT.ProfitOrderID)
	momentumT.ProfitOrderID = ""

	mid := priceTick.Mid
	targetProfit := mid * 0.002

	if trend > momentumT.Threshold {
		momentumT.ActiveOrderID = momentumT.submitOrder(models.Bid, models.Limit, mid+0.01, momentumT.OrderSize)
		momentumT.ProfitOrderID = momentumT.submitOrder(models.Ask, models.Limit, mid+targetProfit, momentumT.OrderSize)
	} else if trend < -momentumT.Threshold {
		momentumT.ActiveOrderID = momentumT.submitOrder(models.Ask, models.Limit, mid-0.01, momentumT.OrderSize)
		momentumT.ProfitOrderID = momentumT.submitOrder(models.Bid, models.Limit, mid-targetProfit, momentumT.OrderSize)
	}
}

// -- NATS request-reply --

func (momentumT *MomentumT) submitOrder(side models.Side, orderType models.OrderType, price float64, quantity float64) string {
	order := models.Order{
		ID:         uuid.New().String(),
		Creator_ID: momentumT.ParticipantConfig.ID,
		Symbol:     momentumT.ParticipantConfig.Symbol,
		Side:       side,
		OrderType:  orderType,
		Price:      price,
		Qty:        quantity,
		CreatedAt:  time.Now(),
	}

	data, err := json.Marshal(order)
	if err != nil {
		fmt.Println("ERROR MARSHALING ORDER IN submitOrder in MOMENTUMT")
		return ""
	}

	msg, err := momentumT.ParticipantConfig.NC.nc.Request(models.OrdersSubmitTopic, data, 2*time.Second)
	if err != nil {
		fmt.Println("ERROR SUBMITTING ORDER IN submitOrder in MOMENTUMT")
		return ""
	}

	var ack models.OrderAck
	if err := json.Unmarshal(msg.Data, &ack); err != nil {
		fmt.Println("ERROR UNMARSHALING ACK IN submitOrder in MOMENTUMT")
		return ""
	}

	return ack.OrderID
}

func (momentumT *MomentumT) cancelOrder(orderID string) {
	if orderID == "" {
		return
	}

	cancelRequest := models.CancelRequest{
		OrderID: orderID,
		Symbol:  momentumT.ParticipantConfig.Symbol,
	}

	data, err := json.Marshal(cancelRequest)
	if err != nil {
		fmt.Println("ERROR MARSHALING CANCEL REQUEST")
		return
	}

	msg, err := momentumT.ParticipantConfig.NC.nc.Request(models.OrdersCancelTopic, data, 2*time.Second)
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
