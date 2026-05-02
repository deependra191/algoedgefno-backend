ALTER TABLE backtest_runs
  ADD COLUMN result_stats jsonb,
  ADD COLUMN chart_data   jsonb;
