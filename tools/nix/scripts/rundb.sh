#!/bin/bash

# Init db
initdb

# Start postgres
pg_ctl start -o "-k '$PGHOST'"

# Create postgres user
createuser -s -r postgres

# Set postgres superuser password
psql -U postgres -c "ALTER ROLE postgres WITH PASSWORD 'postgres';"

# Create databases owned by the role that will run migrations.
# `ledgerline_test` is the throwaway database the Go sync tests truncate.
createdb -O postgres ledgerline
createdb -O postgres ledgerline_test

# Ensure the migrator role can create objects in the default schema
for db in ledgerline ledgerline_test; do
  psql -d "$db" -c "
    CREATE SCHEMA IF NOT EXISTS app;
    GRANT USAGE ON SCHEMA app TO postgres;
  "
done

exit 0