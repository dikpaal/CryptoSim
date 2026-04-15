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

type IndividualTradeTopic string

const (
	ScalperTradeExecutedTopic        IndividualTradeTopic = "mm1.tradeexecuted"
	MomentumTradeExecutedTopic       IndividualTradeTopic = "mm2.tradeexecuted"
	AvstoikovTradeExecutedTopic      IndividualTradeTopic = "mm3.tradeexecuted"
	MomentumChaserTradeExecutedTopic IndividualTradeTopic = "t1.tradeexecuted"
	MeanReversionTradeExecutedTopic  IndividualTradeTopic = "t2.tradeexecuted"
	NoiseTradeExecutedTopic          IndividualTradeTopic = "t3.tradeexecuted"
	VWAPTradeExecutedTopic           IndividualTradeTopic = "t4.tradeexecuted"
)
