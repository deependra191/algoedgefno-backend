-- environment_identity holds exactly one row per database, written once during provisioning.
-- The identity value must match the APP_ENV the backend is configured for.
-- Restore runbooks must rewrite this row after restoring a production backup into staging.
CREATE TABLE IF NOT EXISTS environment_identity (
    identity   VARCHAR(50)  NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
