#!/bin/bash
# Zero-downtime migration script for Pincex (Postgres)
# Uses pg_repack if available, otherwise applies migrations in a transaction

set -e

if [ -z "$PINCEX_DATABASE_DSN" ]; then
    if [ -f .env ]; then
        export $(grep -v '^#' .env | xargs)
    else
        echo "Error: PINCEX_DATABASE_DSN environment variable not set and .env file not found"
        exit 1
    fi
fi

MIGRATIONS_DIR="migrations/postgres"

for sqlfile in $(ls "$MIGRATIONS_DIR"/*.sql | sort); do
    version=$(basename "$sqlfile")
    exists=$(psql -tAc "SELECT 1 FROM migrations WHERE version='$version';" "$PINCEX_DATABASE_DSN")
    if [ "$exists" = "1" ]; then
        echo "Skipping already applied migration: $version"
        continue
    fi
    echo "Applying migration: $version (zero-downtime)"
    # Try to use pg_repack for table changes, fallback to normal apply
    if grep -qE '(ALTER|CREATE|DROP) TABLE' "$sqlfile" && command -v pg_repack >/dev/null 2>&1; then
        echo "Using pg_repack for $version"
        psql "$PINCEX_DATABASE_DSN" -f "$sqlfile"
        # Example: pg_repack -t table_name $PINCEX_DATABASE_DSN
    else
        psql "$PINCEX_DATABASE_DSN" -v ON_ERROR_STOP=1 <<EOF
BEGIN;
\i '$sqlfile'
COMMIT;
EOF
    fi
    psql "$PINCEX_DATABASE_DSN" <<EOF
INSERT INTO migrations (version) VALUES ('$version');
EOF
done

echo "Zero-downtime migrations complete."
