package migrations

import _ "embed"

//go:embed 001_create_trades.sql
var Trades string

//go:embed 002_create_orderbook_snapshots.sql
var OrderbookSnapshots string

//go:embed 003_create_mm_status.sql
var MMStatus string

var All = []string{Trades, OrderbookSnapshots, MMStatus}
