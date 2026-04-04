package models

const (
	PriceBTCTopic = "price.btc"
	PriceXRPTopic = "price.xrp"
	PriceETHTopic = "price.eth"
)

func PriceTopicForSymbol(symbol string) string {
	switch ProductId(symbol) {
	case BTC_USD:
		return PriceBTCTopic
	case ETH_USD:
		return PriceETHTopic
	case XRP_USD:
		return PriceXRPTopic
	default:
		return PriceXRPTopic
	}
}

const (
	TradesExecutedTopic    = "trades.executed"
	OrderBookSnapshotTopic = "orderbook.snapshot"
	OrdersSubmitTopic      = "orders.submit"
	OrdersCancelTopic      = "orders.cancel"
	StatusTopic            = "participant.status"
	MetricsDBTopic         = "metrics.db"
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
