-- Migration: Partition trades table by monthly ranges
-- Filename: 20250701000001_partition_trades.sql

BEGIN;

-- 1. Rename existing table to preserve data
ALTER TABLE public.trades RENAME TO trades_base;

-- 2. Create new parent partitioned table
CREATE TABLE public.trades (
    LIKE trades_base INCLUDING ALL
) PARTITION BY RANGE (created_at);

-- 3. Migrate existing data into new parent (will route to default if set)
INSERT INTO public.trades SELECT * FROM trades_base;

-- 4. Drop old base table
DROP TABLE trades_base;

-- 5. Create monthly partitions for current year and next month
-- Adjust per environment as needed
DO $$
DECLARE
    start_date DATE := date_trunc('month', now()) - INTERVAL '12 months';
    end_date   DATE := date_trunc('month', now()) + INTERVAL '2 months';
    cur_month  DATE;
BEGIN
    cur_month := start_date;
    WHILE cur_month < end_date LOOP
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS public.trades_%s PARTITION OF public.trades
             FOR VALUES FROM (''%s'') TO (''%s'');',
            to_char(cur_month, 'YYYY_MM'),
            to_char(cur_month, 'YYYY-MM-01'),
            to_char(cur_month + INTERVAL '1 month', 'YYYY-MM-01')
        );
        cur_month := cur_month + INTERVAL '1 month';
    END LOOP;
END $$;

-- 6. (Optional) Create default partition for out-of-range data
CREATE TABLE IF NOT EXISTS public.trades_default PARTITION OF public.trades DEFAULT;

COMMIT;
