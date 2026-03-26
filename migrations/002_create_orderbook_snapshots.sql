CREATE TABLE IF NOT EXISTS orderbook_snapshots (
      id          BIGSERIAL,
      symbol      TEXT NOT NULL,
      bids        JSONB NOT NULL,
      asks        JSONB NOT NULL,
      mid_price   NUMERIC(20, 8),
      spread      NUMERIC(20, 8),
      snapshot_at TIMESTAMPTZ NOT NULL
  );

  SELECT create_hypertable('orderbook_snapshots', 'snapshot_at');

  CREATE INDEX ON orderbook_snapshots (symbol, snapshot_at DESC);