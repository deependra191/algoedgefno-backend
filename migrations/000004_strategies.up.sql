CREATE TABLE strategies (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                 TEXT    NOT NULL,
    description          TEXT    NOT NULL DEFAULT '',
    underlying           TEXT    NOT NULL,
    instrument_type      TEXT    NOT NULL,
    expiry_rule          TEXT    NOT NULL DEFAULT 'CURRENT_MONTH',
    option_leg_json      JSONB,
    entry_condition_type TEXT    NOT NULL,
    target_pct           NUMERIC,
    stop_loss_pct        NUMERIC,
    time_exit_minutes    INTEGER,
    lot_size             INTEGER NOT NULL DEFAULT 1,
    capital_per_trade    NUMERIC,
    mode                 TEXT    NOT NULL DEFAULT 'PAPER',
    is_ready_for_run     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
