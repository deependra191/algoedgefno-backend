ALTER TABLE backtest_runs ALTER COLUMN strategy_id DROP NOT NULL;

ALTER TABLE backtest_runs ADD COLUMN strategy_slug TEXT;
ALTER TABLE backtest_runs ADD COLUMN capital NUMERIC;
ALTER TABLE backtest_runs ADD COLUMN lots INTEGER;
ALTER TABLE backtest_runs ADD COLUMN underlying TEXT;

ALTER TABLE backtest_runs ADD CONSTRAINT chk_strategy_ref
    CHECK (strategy_id IS NOT NULL OR strategy_slug IS NOT NULL);

CREATE INDEX idx_backtest_runs_slug ON backtest_runs(strategy_slug)
    WHERE strategy_slug IS NOT NULL;
