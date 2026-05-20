ALTER TABLE backtest_runs
  ADD COLUMN slippage NUMERIC;

WITH slippage_backfill AS (
  SELECT
    b.id,
    SUM(COALESCE(NULLIF(trade.item->>'Slippage', '')::NUMERIC, 0)) AS total_slippage
  FROM backtest_runs b
  CROSS JOIN LATERAL jsonb_array_elements(
    CASE
      WHEN jsonb_typeof(b.trades_json) = 'array' THEN b.trades_json
      ELSE '[]'::jsonb
    END
  ) AS trade(item)
  WHERE b.trades_json IS NOT NULL
  GROUP BY b.id
)
UPDATE backtest_runs b
SET
  slippage = sb.total_slippage,
  total_charges = b.total_charges - sb.total_slippage
FROM slippage_backfill sb
WHERE b.id = sb.id
  AND b.total_charges IS NOT NULL
  AND sb.total_slippage > 0;
