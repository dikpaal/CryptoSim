package models

type PriceTick struct {
	Symbol    string  `json:"symbol"`
	Bid       float64 `json:"bid"`
	Ask       float64 `json:"ask"`
	Mid       float64 `json:"mid"`
	BidSize   float64 `json:"bid_size"` // best_bid_quantity
	AskSize   float64 `json:"ask_size"` // best_ask_quantity
	Price     float64 `json:"price"`
	Timestamp int64   `json:"timestamp"`
}
