package mm

type PriceTick struct {
	Symbol    string  `json:"symbol"`
	Bid       float64 `json:"bid"`
	Ask       float64 `json:"ask"`
	Mid       float64 `json:"mid"`
	Timestamp int64   `json:"timestamp"`
}

type Quote struct {
	BidPrice float64
	BidQty   float64
	AskPrice float64
	AskQty   float64
}

type Strategy interface {
	OnPriceTick(tick PriceTick, inventory float64) *Quote
	Name() string
}
