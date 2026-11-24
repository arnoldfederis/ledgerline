-- name: GetBudgetVsActual :many
-- One row per category for one calendar month.
--
-- Actuals are SUM(debit) - SUM(credit), so a refund reduces spend instead of
-- being stored as a negative debit.
--
-- The LEFT JOINs keep a budgeted category with no spend (under budget by the
-- full amount) and a category with spend and no budget (unbudgeted, the case
-- worth flagging). Only categories with neither are dropped.
WITH
  actuals AS (
    SELECT
      t.category_id,
      SUM(
        CASE
          WHEN t.direction = 'debit' THEN t.amount_cents
          ELSE - t.amount_cents
        END
      )::bigint AS actual_cents,
      COUNT(*)::bigint AS transaction_count
    FROM
      app.transactions t
    WHERE
      t.voided_at IS NULL
      AND t.occurred_on >= sqlc.arg(period_month)::date
      AND t.occurred_on < (sqlc.arg(period_month)::date + make_interval(months => 1))
    GROUP BY
      t.category_id
  )
SELECT
  c.code AS category_code,
  c.name AS category_name,
  COALESCE(b.amount_cents, 0)::bigint AS budget_cents,
  COALESCE(a.actual_cents, 0)::bigint AS actual_cents,
  COALESCE(a.transaction_count, 0)::bigint AS transaction_count,
  (b.id IS NOT NULL)::boolean AS has_budget
FROM
  app.categories c
  LEFT JOIN app.budgets b ON b.category_id = c.id
  AND b.period_month = sqlc.arg(period_month)::date
  LEFT JOIN actuals a ON a.category_id = c.id
WHERE
  b.id IS NOT NULL
  OR a.category_id IS NOT NULL
ORDER BY
  c.name;

-- name: ListReportableMonths :many
-- Months with a budget, with activity, or both.
SELECT
  MONTH::date AS period_month
FROM
  (
    SELECT
      period_month AS MONTH
    FROM
      app.budgets
    UNION
    SELECT
      date_trunc('month', occurred_on)::date
    FROM
      app.transactions
    WHERE
      voided_at IS NULL
  ) AS months
ORDER BY
  MONTH DESC;
