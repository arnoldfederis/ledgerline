# Development environment

How to get the monorepo running on a new machine. For what Ledgerline is and
how it works, see the [README](../README.md).

## Stack

- Monorepo: Turborepo
- Backend: Golang
- Frontend: React + Vite
- Package Manager: Bun
- Database: PostgreSQL
- Development Environment: Devbox

#### PS: When running custom script, make sure you are in the project root directory

## Verify Prerequisites

```sh
nix --version
direnv --version
devbox version
```

## Init local

- Open the terminal, install the devbox if not yet installed:

```bash
curl -fsSL https://get.jetify.com/devbox | bash
```

## Set up as a new project

- Same terminal or new terminal and go to the project root

```bash
cd <project-root>

## install devbox
devbox install

## close the terminal and open a new one
## if direnv is not yet allowed run
direnv allow

## run the env-decrypt script
env-decrypt

# Rename all instances of `ledgerline` to your project name
# then proceed to Getting Started

## run the init-local script
init-local
```

## Init local manually

- Open the terminal, install the devbox if not yet installed:

```bash
curl -fsSL https://get.jetify.com/devbox | bash
```

## Set up as a new project

- Same terminal or new terminal and go to the project root

```bash
cd <project-root>

## install devbox
devbox install

## close the terminal and open a new one
## if direnv is not yet allowed run
direnv allow

## run the env-decrypt script
env-decrypt

# Rename all instances of `ledgerline` to your project name
# then proceed to Getting Started
```

- Install dependencies:

```bash
bun install
# or
bun i

bun run tidy
```

- Generate cert file

```bash
cert-gen
```

- Sync env

```aiignore
## run env-decrypt script
## it will decrypt and create a file in `tools/secrets/apps.local.yaml`
env-decrypt

## then run env-sync script
## it will create .env file to the designated directory
env-sync

## if you have new changes in the `tools/secrets/apps.local.yaml` file

## run the env-encrypt script
env-encrypt

## then push the changes to the repo
## so that the encrypted file can be used by other developers
```

- Sync hosts

```bash
sync-hosts
```

- Run the development services:

```bash
dev up
```

## Reset your local

```bash
## Open terminal and go to the project root
## run the following commands
reset-local
```

## View logs of the running services

```bash
## more info of running services is in `process-compose.yaml`
dev logs <service-name>

## example
dev logs postgresql
```

## Add a package to the project

```bash
cd <package-path-folder>
bun add <package-name>

## example
bun add pg

## or use the custom script
bunadd pg <package-path-folder>

## example
bunadd pg apps/site
```

## Add a dev package to the project

```bash
cd <package-path-folder>
bun add -D <package-name>

## example
bun add -D pg

## or use the custom script
bundevadd pg <package-path-folder>

## example
bundevadd pg apps/web
```

## Remove a package from the project

```bash
cd <package-path-folder>
bun rm <package-name>

## example
cd apps/web
bun rm pg

## or use the custom script
bunrm pg <package-path-folder>

## example
bunrm pg apps/web
```

## Check an outdated package

```bash
bun outdated -r
```

## Update outdated package

```bash
bun update --interactive --recursive
# or
bun update -i -r
```

## Go workflow

The Go toolchain, `sqlc` and `goose` all come from Devbox, so there is nothing
to install globally.

```bash
## regenerate the type-safe query layer after editing
## apps/server/internal/database/queries/*.sql
bun run generate -F @repo/server

## apply migrations by hand (the server also does this on boot
## when DB_AUTO_MIGRATE=true)
bun run migrate -F @repo/server

## roll the most recent migration back
bun run migrate:down -F @repo/server

## print the schema version recorded in the database
bun run migrate:version -F @repo/server
```

### Adding a migration

```bash
cd apps/server/internal/database/migrations
goose create add_something_useful sql
```

Then edit the generated file, run `bun run generate -F @repo/server` so sqlc
type-checks the queries against the new schema, and `bun run migrate -F
@repo/server` to apply it.

### Tests

```bash
## everything
bun run test

## Go only
bun run test -F @repo/server

## web only
bun run test -F @repo/web
```

The sync tests need a throwaway PostgreSQL database. `rundb` creates
`ledgerline_test` alongside `ledgerline`, and `TEST_DATABASE_URL` in
`apps/server/.env` points at it; the tests truncate it on every run and skip
themselves entirely if the variable is not set.

### Regenerating the mock ledger

The fixture the mock accounting API serves is committed at
`apps/server/internal/mocksource/data/transactions.json`. To change its shape
(budgets, categories, how far over budget a category runs), edit
`data/generate.go` and re-run it:

```bash
cd apps/server && go generate ./...
```

The generator builds the fixture through the same `accounting.WireTransaction`
type the sync job parses, so the two cannot drift, and it prints which
categories end up over budget. Keep its `budgets` table in step with
`apps/server/internal/database/migrations/00002_seed_budgets.sql`.

## Formatting

One formatter per language, all wired into the same root scripts:

```bash
## everything: gofmt, oxfmt, and the SQL formatter
bun run format

## SQL only
bun run format:sql

## check without writing -- what CI should run
bun run format:sql:check
```

SQL is formatted by `tools/scripts/format-sql.mjs` (sql-formatter, PostgreSQL
dialect, 2-space indent, uppercase keywords). It covers every `.sql` file in
the repo, so migrations and sqlc queries stay consistent.

Two project-specific rules live in that script:

- `paramTypes: { named: ["@"] }` keeps sqlc placeholders intact. Without it the
  formatter writes `@ source` and sqlc stops recognising the parameter.
- `sqlc.narg (x)` is collapsed back to `sqlc.narg(x)`.

Migrations deliberately do **not** use `-- +goose StatementBegin` /
`StatementEnd`. Those are only needed for statements containing their own
semicolons (functions, `DO` blocks); plain DDL and DML split correctly without
them, and leaving them out keeps the files formattable.

## Screenshots

The README images are captured from the running dev server by headless Chrome,
so they come out at 2x device pixel ratio instead of whatever a screenshot tool
happens to give you:

```bash
## with `dev up` running
bun run screenshots
```

That writes `docs/dashboard-dark.png` and `docs/dashboard-light.png` at
2360x2680. The theme is pinned with `?theme=dark|light`, which the boot script
in `index.html` honours ahead of localStorage handy for sharing a link in a
specific theme, and what makes the capture reproducible.

Resolution is the reason for the extra machinery: downscaling a 1x capture
cannot recover pixels, so the page has to be _rendered_ at 2x in the first
place (`--force-device-scale-factor=2`).
