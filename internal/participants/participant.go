package participants

type ParticipantConfig struct {
	ID       string
	Symbol   string
	NC       *NATSConn
	MidPrice float64 // latest from prices.{symbol}
	Position float64 // current inventory
	Cash     float64 // available cash
}
