-- name: UpsertCategory :one
-- Called for every transaction's category. `code` is the natural key, so an
-- upstream rename updates the label in place instead of splitting the budget
-- across two categories.
INSERT INTO
  app.categories (code, name)
VALUES
  (@code, @name)
ON CONFLICT (code) DO UPDATE
SET
  name = excluded.name,
  updated_at = now()
RETURNING
  *;

-- name: ListCategories :many
SELECT
  *
FROM
  app.categories
ORDER BY
  name;
