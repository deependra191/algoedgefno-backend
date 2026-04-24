DROP INDEX IF EXISTS idx_backtest_runs_slug;
ALTER TABLE backtest_runs DROP CONSTRAINT IF EXISTS chk_strategy_ref;
ALTER TABLE backtest_runs DROP COLUMN IF EXISTS underlying;
ALTER TABLE backtest_runs DROP COLUMN IF EXISTS lots;
ALTER TABLE backtest_runs DROP COLUMN IF EXISTS capital;
ALTER TABLE backtest_runs DROP COLUMN IF EXISTS strategy_slug;

DELETE FROM backtest_runs WHERE strategy_id IS NULL;
ALTER TABLE backtest_runs ALTER COLUMN strategy_id SET NOT NULL;
