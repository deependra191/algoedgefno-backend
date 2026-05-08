ALTER TABLE backtest_runs
  ADD COLUMN signal_instrument_token TEXT;

COMMENT ON COLUMN backtest_runs.instrument_token IS 'Trade-side instrument token used for execution and canonical run metadata.';
COMMENT ON COLUMN backtest_runs.signal_instrument_token IS 'Signal-side instrument token used only for signal candle selection; nullable for legacy rows.';
