CREATE TABLE sync_runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider          TEXT        NOT NULL,
    sync_type         TEXT        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'RUNNING',
    records_processed INTEGER     NOT NULL DEFAULT 0,
    error_message     TEXT,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at      TIMESTAMPTZ
);

CREATE INDEX idx_sync_runs_provider_started ON sync_runs(provider, started_at DESC);
