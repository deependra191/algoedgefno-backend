ALTER TABLE backtest_runs
  ADD COLUMN slippage NUMERIC;

WITH slippage_backfill AS (
  SELECT
    trade_rows.id,
    SUM(trade_rows.trade_slippage) AS total_slippage,
    jsonb_agg(
      CASE
        WHEN trade_rows.has_total_charges_key AND trade_rows.has_slippage_key THEN
          CASE
            WHEN trade_rows.item ? 'TotalCharges' THEN
              jsonb_set(
                trade_rows.item,
                '{TotalCharges}',
                to_jsonb(trade_rows.trade_total_charges - trade_rows.trade_slippage),
                false
              )
            WHEN trade_rows.item ? 'totalCharges' THEN
              jsonb_set(
                trade_rows.item,
                '{totalCharges}',
                to_jsonb(trade_rows.trade_total_charges - trade_rows.trade_slippage),
                false
              )
            ELSE trade_rows.item
          END
        ELSE trade_rows.item
      END
      ORDER BY trade_rows.ordinality
    ) AS trades_json
  FROM (
    SELECT
      b.id,
      trade.ordinality,
      trade.item,
      COALESCE(NULLIF(trade.item->>'Slippage', '')::NUMERIC, NULLIF(trade.item->>'slippage', '')::NUMERIC, 0) AS trade_slippage,
      COALESCE(NULLIF(trade.item->>'TotalCharges', '')::NUMERIC, NULLIF(trade.item->>'totalCharges', '')::NUMERIC, 0) AS trade_total_charges,
      (trade.item ? 'Slippage' OR trade.item ? 'slippage') AS has_slippage_key,
      (trade.item ? 'TotalCharges' OR trade.item ? 'totalCharges') AS has_total_charges_key
    FROM backtest_runs b
    CROSS JOIN LATERAL jsonb_array_elements(
      CASE
        WHEN jsonb_typeof(b.trades_json) = 'array' THEN b.trades_json
        ELSE '[]'::jsonb
      END
    ) WITH ORDINALITY AS trade(item, ordinality)
    WHERE b.trades_json IS NOT NULL
  ) AS trade_rows
  GROUP BY trade_rows.id
  HAVING BOOL_OR(trade_rows.has_slippage_key)
)
UPDATE backtest_runs b
SET
  slippage = sb.total_slippage,
  total_charges = CASE
    WHEN b.total_charges IS NULL THEN NULL
    ELSE b.total_charges - sb.total_slippage
  END,
  trades_json = sb.trades_json
FROM slippage_backfill sb
WHERE b.id = sb.id;
