package models

type Side string
type OrderType string
type OrderStatus string
type WSRequestType string
type ProductId string
type Channel string

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

// SUBSCRIBE | UNSUBSCRIBE
const (
	Subscribe   WSRequestType = "subscribe"
	Unsubscribe WSRequestType = "unsubscribe"
)

const (
	BTC_USD ProductId = "BTC-USD"
	XRP_USD ProductId = "XRP-USD"
	ETH_USD ProductId = "ETH-USD"
)

const (
	Ticker Channel = "ticker"
)
