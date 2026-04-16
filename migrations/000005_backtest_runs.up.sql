CREATE TABLE backtest_runs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    strategy_id      UUID        NOT NULL REFERENCES strategies(id),
    instrument_token TEXT        NOT NULL,
    from_ts          TIMESTAMPTZ NOT NULL,
    to_ts            TIMESTAMPTZ NOT NULL,
    candle_interval  TEXT        NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'PENDING',
    net_pnl          NUMERIC,
    total_trades     INTEGER,
    win_count        INTEGER,
    loss_count       INTEGER,
    max_drawdown     NUMERIC,
    trades_json      JSONB,
    error_message    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at     TIMESTAMPTZ
);

CREATE INDEX idx_backtest_runs_strategy ON backtest_runs(strategy_id);
