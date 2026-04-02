package models

type OrderbookSnapshot struct {
	Symbol string       `json:"symbol"`
	Bids   [][2]float64 `json:"bids"`
	Asks   [][2]float64 `json:"asks"`
}
