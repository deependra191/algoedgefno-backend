WITH charge_restore AS (
  SELECT
    trade_rows.id,
    jsonb_agg(
      CASE
        WHEN trade_rows.has_total_charges_key AND trade_rows.has_slippage_key THEN
          CASE
            WHEN trade_rows.item ? 'TotalCharges' THEN
              jsonb_set(
                trade_rows.item,
                '{TotalCharges}',
                to_jsonb(trade_rows.trade_total_charges + trade_rows.trade_slippage),
                false
              )
            WHEN trade_rows.item ? 'totalCharges' THEN
              jsonb_set(
                trade_rows.item,
                '{totalCharges}',
                to_jsonb(trade_rows.trade_total_charges + trade_rows.trade_slippage),
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
SET trades_json = cr.trades_json
FROM charge_restore cr
WHERE b.id = cr.id;

UPDATE backtest_runs
SET total_charges = total_charges + slippage
WHERE total_charges IS NOT NULL
  AND slippage IS NOT NULL;

ALTER TABLE backtest_runs
  DROP COLUMN slippage;
