package models

type DBMetrics struct {
	TradeWrites uint64 `json:"trade_writes"`
	TotalWrites uint64 `json:"total_writes"`
}
