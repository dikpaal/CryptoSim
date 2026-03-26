CREATE TABLE IF NOT EXISTS trades (
      trade_id        UUID PRIMARY KEY,
      symbol          TEXT NOT NULL,
      price           NUMERIC(20, 8) NOT NULL,
      qty             NUMERIC(20, 8) NOT NULL,
      buyer_mm_id     TEXT NOT NULL,
      seller_mm_id    TEXT NOT NULL,
      buyer_order_id  UUID NOT NULL,
      seller_order_id UUID NOT NULL,
      executed_at     TIMESTAMPTZ NOT NULL
  );

  SELECT create_hypertable('trades', 'executed_at');