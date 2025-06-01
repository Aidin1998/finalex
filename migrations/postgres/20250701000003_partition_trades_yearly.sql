-- Migration: Create yearly partitions for trades table (current ±1 year)
-- Filename: 20250701000003_partition_trades_yearly.sql

BEGIN;

-- Ensure trades is parent partitioned table
ALTER TABLE public.trades
  PARTITION BY RANGE (created_at);

-- Generate yearly partitions
DO $$
DECLARE
    current_year INT := date_part('year', now());
    year_offset  INT;
    start_date   DATE;
    next_year    DATE;
BEGIN
    FOR year_offset IN -1..1 LOOP
        start_date := make_date(current_year + year_offset, 1, 1);
        next_year  := make_date(current_year + year_offset + 1, 1, 1);
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS public.trades_%s PARTITION OF public.trades
             FOR VALUES FROM (''%s'') TO (''%s'');',
            to_char(start_date, 'YYYY'),
            to_char(start_date, 'YYYY-MM-DD'),
            to_char(next_year,  'YYYY-MM-DD')
        );
    END LOOP;
END $$;

COMMIT;
