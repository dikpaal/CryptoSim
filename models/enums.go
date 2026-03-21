package models

type Side string
type OrderType string
type OrderStatus string

// BID | ASK
const (
	Bid Side = "BID"
	Ask Side = "ASK"
)

// LIMIT | MARKET
const (
	Limit  OrderType = "LIMIT"
	Market OrderType = "MARKET"
)

// PENDING | PARTIAL | FILLED | CANCELLED
const (
	Pending   OrderStatus = "PENDING"
	Partial   OrderStatus = "PARTIAL"
	Filled    OrderStatus = "FILLED"
	Cancelled OrderStatus = "CANCELLED"
)
