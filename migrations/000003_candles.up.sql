CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE candles (
    instrument_id UUID        NOT NULL REFERENCES instruments(id),
    ts            TIMESTAMPTZ NOT NULL,
    interval      TEXT        NOT NULL,
    open          NUMERIC     NOT NULL,
    high          NUMERIC     NOT NULL,
    low           NUMERIC     NOT NULL,
    close         NUMERIC     NOT NULL,
    volume        BIGINT      NOT NULL DEFAULT 0,
    provider      TEXT        NOT NULL,
    PRIMARY KEY (instrument_id, ts, interval)
);

SELECT create_hypertable('candles', 'ts');

CREATE INDEX idx_candles_instrument_interval ON candles(instrument_id, interval, ts DESC);
