-- Migration: Automate partition creation for orders table by monthly ranges
-- Filename: 20250701000002_partition_orders_dynamic.sql

BEGIN;

-- Ensure orders is partitioned by RANGE on created_at
ALTER TABLE public.orders
    PARTITION BY RANGE (created_at);

-- Create monthly partitions for past 12 months and next 2 months dynamically
DO $$
DECLARE
    start_date DATE := date_trunc('month', now()) - INTERVAL '12 months';
    end_date   DATE := date_trunc('month', now()) + INTERVAL '2 months';
    cur_month  DATE;
BEGIN
    cur_month := start_date;
    WHILE cur_month < end_date LOOP
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS public.orders_%s PARTITION OF public.orders
             FOR VALUES FROM (''%s'') TO (''%s'');',
            to_char(cur_month, 'YYYY_MM'),
            to_char(cur_month, 'YYYY-MM-01'),
            to_char(cur_month + INTERVAL '1 month', 'YYYY-MM-01')
        );
        cur_month := cur_month + INTERVAL '1 month';
    END LOOP;
END $$;

-- Create default partition to capture any out-of-range data
CREATE TABLE IF NOT EXISTS public.orders_default PARTITION OF public.orders DEFAULT;

COMMIT;
