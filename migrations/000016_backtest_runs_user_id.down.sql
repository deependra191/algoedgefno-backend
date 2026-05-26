DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM backtest_runs WHERE user_id IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot drop backtest_runs.user_id: % rows have user_id assigned',
            (SELECT COUNT(*) FROM backtest_runs WHERE user_id IS NOT NULL);
    END IF;
END$$;

DROP INDEX IF EXISTS idx_backtest_runs_user_id;
ALTER TABLE backtest_runs DROP COLUMN IF EXISTS user_id;
