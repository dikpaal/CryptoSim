package models

import "time"

type OrderbookSnapshot struct {
	Symbol     string
	Bids       [][2]float64
	Asks       [][2]float64
	MidPrice   float64
	Spread     float64
	SnapshotAt time.Time
}
