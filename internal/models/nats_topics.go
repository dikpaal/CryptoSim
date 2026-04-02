package models

const (
	PriceBTCTopic = "price.btc"
	PriceXRPTopic = "price.xrp"
	PriceETHTopic = "price.eth"
)

const (
	OrderBookSnapshotTopic = "orderbook.snapshot"
	OrdersSubmitTopic      = "orders.submit"
	OrdersCancelTopic      = "orders.cancel"
	StatusTopic            = "participant.status"
)

const (
	ScalperTradeExecutedTopic        = "mm1.tradeexecuted"
	MomentumTradeExecutedTopic       = "mm2.tradeexecuted"
	AvstoikovTradeExecutedTopic      = "mm3.tradeexecuted"
	MomentumChaserTradeExecutedTopic = "t1.tradeexecuted"
	MeanReversionTradeExecutedTopic  = "t2.tradeexecuted"
	NoiseTradeExecutedTopic          = "t3.tradeexecuted"
	VWAPTradeExecutedTopic           = "t4.tradeexecuted"
)
