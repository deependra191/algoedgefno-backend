COMMENT ON COLUMN backtest_runs.instrument_token IS NULL;

-- Rollback drops signal-side token data; acceptable for v1 single-user deployments.
ALTER TABLE backtest_runs
  DROP COLUMN signal_instrument_token;
