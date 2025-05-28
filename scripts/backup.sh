#!/usr/bin/env bash
# backup.sh: Automated backup pipeline for PostgreSQL and CockroachDB
# Usage: Ensure environment variables PG_DSN and CR_DSN are set.
set -eo pipefail

# Timestamp for backup files
TIMESTAMP=$(date +"%Y%m%d%H%M%S")

# Directories
BACKUP_DIR="backups"
PG_DIR="$BACKUP_DIR/postgres"
CR_DIR="$BACKUP_DIR/cockroach"

# Ensure backup directories exist
mkdir -p "$PG_DIR" "$CR_DIR"

echo "[Backup] Starting PostgreSQL backup at $TIMESTAMP"
# PostgreSQL backup (SQL dump)
pg_dump "$PG_DSN" > "$PG_DIR/backup_$TIMESTAMP.sql"
echo "[Backup] PostgreSQL backup saved to $PG_DIR/backup_$TIMESTAMP.sql"

echo "[Backup] Starting CockroachDB dump at $TIMESTAMP"
# CockroachDB backup (SQL dump)
cockroach dump --url "$CR_DSN" > "$CR_DIR/backup_$TIMESTAMP.sql"
echo "[Backup] CockroachDB dump saved to $CR_DIR/backup_$TIMESTAMP.sql"

# Cleanup backups older than 7 days
echo "[Backup] Cleaning up old backups older than 7 days"
find "$BACKUP_DIR" -type f -mtime +7 -exec rm -f {} +
echo "[Backup] Backup pipeline completed."
