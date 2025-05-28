#!/bin/bash
# Partition maintenance script for Pincex (Postgres)
# Creates future monthly partitions for orders and trades tables

set -e

if [ -z "$PINCEX_DATABASE_DSN" ]; then
    if [ -f .env ]; then
        export $(grep -v '^#' .env | xargs)
    else
        echo "Error: PINCEX_DATABASE_DSN environment variable not set and .env file not found"
        exit 1
    fi
fi

# How many months ahead to create partitions for
MONTHS_AHEAD=6

for table in orders trades; do
    for i in $(seq 0 $MONTHS_AHEAD); do
        PARTITION_DATE=$(date -d "+$i month" +"%Y_%m")
        START_DATE=$(date -d "+$i month" +"%Y-%m-01")
        END_DATE=$(date -d "+$((i+1)) month" +"%Y-%m-01")
        PARTITION_NAME="${table}_p${PARTITION_DATE}"
        EXISTS=$(psql "$PINCEX_DATABASE_DSN" -tAc "SELECT 1 FROM pg_tables WHERE tablename = '$PARTITION_NAME';")
        if [ "$EXISTS" = "1" ]; then
            echo "Partition $PARTITION_NAME already exists, skipping."
            continue
        fi
        echo "Creating partition $PARTITION_NAME for $table ($START_DATE to $END_DATE)"
        psql "$PINCEX_DATABASE_DSN" <<EOF
CREATE TABLE IF NOT EXISTS $PARTITION_NAME PARTITION OF $table
    FOR VALUES FROM ('$START_DATE') TO ('$END_DATE');
EOF
    done
done

echo "Partition maintenance complete."
