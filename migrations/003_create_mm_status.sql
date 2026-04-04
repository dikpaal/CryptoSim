CREATE TABLE IF NOT EXISTS mm_status (
      id              BIGSERIAL,
      mm_id           TEXT NOT NULL,
      strategy        TEXT NOT NULL,
      inventory       NUMERIC(20, 8) NOT NULL,
      realized_pnl    NUMERIC(20, 8) NOT NULL,
      unrealized_pnl  NUMERIC(20, 8) NOT NULL,
      open_orders     INT NOT NULL,
      recorded_at     TIMESTAMPTZ NOT NULL
  );

  SELECT create_hypertable('mm_status', 'recorded_at', if_not_exists => TRUE);

  CREATE INDEX ON mm_status (mm_id, recorded_at DESC);