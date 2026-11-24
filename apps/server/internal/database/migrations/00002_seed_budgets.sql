-- +goose Up
-- Reference data, not user data: the chart of accounts and the approved budget
-- for the demo window. Transactions are never seeded -- the sync job is the
-- only way actuals get in -- so the sync path runs from the first boot.
--
-- Written as upserts, so re-running against a populated database is a no-op.
INSERT INTO
  app.categories (code, name)
VALUES
  ('software', 'Software & Subscriptions'),
  ('payroll', 'Payroll & Contractors'),
  ('cloud', 'Cloud & Hosting'),
  ('marketing', 'Marketing & Advertising'),
  ('travel', 'Travel & Meals'),
  ('office', 'Office & Supplies'),
  ('professional-services', 'Professional Services'),
  ('banking', 'Banking & Fees')
ON CONFLICT (code) DO UPDATE
SET
  name = excluded.name,
  updated_at = now();

-- Keep in step with internal/mocksource/data/generate.py.
INSERT INTO
  app.budgets (category_id, period_month, amount_cents, currency)
SELECT
  c.id,
  b.period_month,
  b.amount_cents,
  'USD'
FROM
  (
    VALUES
      ('software', DATE '2026-09-01', 250000),
      ('software', DATE '2026-10-01', 250000),
      ('software', DATE '2026-11-01', 275000),
      ('payroll', DATE '2026-09-01', 1150000),
      ('payroll', DATE '2026-10-01', 1150000),
      ('payroll', DATE '2026-11-01', 1200000),
      ('cloud', DATE '2026-09-01', 180000),
      ('cloud', DATE '2026-10-01', 180000),
      ('cloud', DATE '2026-11-01', 195000),
      ('marketing', DATE '2026-09-01', 600000),
      ('marketing', DATE '2026-10-01', 650000),
      ('marketing', DATE '2026-11-01', 650000),
      ('travel', DATE '2026-09-01', 320000),
      ('travel', DATE '2026-10-01', 320000),
      ('travel', DATE '2026-11-01', 280000),
      ('office', DATE '2026-09-01', 90000),
      ('office', DATE '2026-10-01', 90000),
      ('office', DATE '2026-11-01', 90000),
      ('professional-services', DATE '2026-09-01', 450000),
      ('professional-services', DATE '2026-10-01', 450000),
      ('professional-services', DATE '2026-11-01', 500000),
      ('banking', DATE '2026-09-01', 45000),
      ('banking', DATE '2026-10-01', 45000),
      ('banking', DATE '2026-11-01', 45000)
  ) AS b (code, period_month, amount_cents)
  JOIN app.categories c ON c.code = b.code
ON CONFLICT (category_id, period_month) DO UPDATE
SET
  amount_cents = excluded.amount_cents,
  updated_at = now();

-- +goose Down
DELETE FROM app.budgets
WHERE
  period_month IN (DATE '2026-09-01', DATE '2026-10-01', DATE '2026-11-01');

DELETE FROM app.categories
WHERE
  code IN (
    'software',
    'payroll',
    'cloud',
    'marketing',
    'travel',
    'office',
    'professional-services',
    'banking'
  );
