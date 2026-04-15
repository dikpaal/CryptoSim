package models

type TradeAck struct {
	TradeID string `json:"trade_id"`
	Reason  string `json:"reason,omitempty"`
}
