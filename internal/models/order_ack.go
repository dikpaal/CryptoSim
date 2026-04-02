package models

type OrderAck struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"` // "accepted" or "rejected"
	Reason  string `json:"reason,omitempty"`
}
