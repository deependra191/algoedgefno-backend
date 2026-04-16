CREATE TABLE instruments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    symbol          TEXT NOT NULL,
    name            TEXT NOT NULL,
    exchange        TEXT NOT NULL DEFAULT 'NFO',
    instrument_type TEXT NOT NULL,
    underlying      TEXT,
    expiry          DATE,
    strike          NUMERIC,
    option_type     TEXT,
    lot_size        INTEGER NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(symbol, exchange)
);

CREATE INDEX idx_instruments_underlying ON instruments(underlying);
CREATE INDEX idx_instruments_exchange_type ON instruments(exchange, instrument_type);
