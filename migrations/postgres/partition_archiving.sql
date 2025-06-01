-- Maintenance: Detach and archive partitions older than 6 months
-- Filename: partition_archiving.sql

-- This script detaches partitions older than the retention period (6 months) and moves them to `historical` schema.

BEGIN;

-- Adjust schema for historical data
CREATE SCHEMA IF NOT EXISTS historical;

-- Loop through old monthly partitions
DO $$
DECLARE
    cutoff_date DATE := date_trunc('month', now()) - INTERVAL '6 months';
    partition_name TEXT;
    partition_schema TEXT := 'public';
BEGIN
    FOR partition_name IN
        SELECT relname
        FROM pg_class c
        JOIN pg_inherits i ON c.oid = i.inhrelid
        JOIN pg_class p ON p.oid = i.inhparent
        WHERE p.relname = 'trades'
          AND to_date(split_part(c.relname, '_', 2)||'-01', 'YYYY-MM-DD') < cutoff_date
    LOOP
        -- Detach partition
        EXECUTE format('ALTER TABLE public.trades DETACH PARTITION %I', partition_name);
        -- Move to historical schema
        EXECUTE format('ALTER TABLE public.%I SET SCHEMA historical', partition_name);
    END LOOP;
END $$;

COMMIT;
