DROP INDEX IF EXISTS idx_backtest_runs_slug;
ALTER TABLE backtest_runs DROP CONSTRAINT IF EXISTS chk_strategy_ref;
ALTER TABLE backtest_runs DROP COLUMN IF EXISTS underlying;
ALTER TABLE backtest_runs DROP COLUMN IF EXISTS lots;
ALTER TABLE backtest_runs DROP COLUMN IF EXISTS capital;
ALTER TABLE backtest_runs DROP COLUMN IF EXISTS strategy_slug;

-- Rolling back requires manual triage: if any slug-based runs exist, SET NOT NULL will fail.
-- Inspect and handle those rows before re-attempting the migration down.
ALTER TABLE backtest_runs ALTER COLUMN strategy_id SET NOT NULL;
