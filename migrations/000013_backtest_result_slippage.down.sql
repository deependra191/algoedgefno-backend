UPDATE backtest_runs
SET total_charges = total_charges + slippage
WHERE total_charges IS NOT NULL
  AND slippage IS NOT NULL;

ALTER TABLE backtest_runs
  DROP COLUMN slippage;
