CREATE TABLE IF NOT EXISTS trades (
      trade_id        UUID,
      symbol          TEXT NOT NULL,
      price           NUMERIC(20, 8) NOT NULL,
      qty             NUMERIC(20, 8) NOT NULL,
      buyer_mm_id     TEXT NOT NULL,
      seller_mm_id    TEXT NOT NULL,
      buyer_order_id  UUID NOT NULL,
      seller_order_id UUID NOT NULL,
      executed_at     TIMESTAMPTZ NOT NULL,
      PRIMARY KEY (trade_id, executed_at)
  );

  SELECT create_hypertable('trades', 'executed_at', if_not_exists => TRUE);