# Database Partitioning and Scalability Guide

This document outlines the strategy, implementation, and maintenance processes for monthly (and yearly) partitioning of large transactional tables (e.g., `orders`, `trades`) to improve performance, maintainability, and ACID compliance.

## 1. Partitioning Strategy

- **Range Partitioning by Month**: Transactions are partitioned on the `created_at` timestamp by monthly ranges to evenly distribute data and facilitate efficient pruning.
- **Dynamic Partition Creation**: A PL/pgSQL script runs at startup or via scheduled job to create new partitions automatically for the next 2 months and remove old partitions if desired.
- **Default Partition**: A default partition captures out-of-range data to prevent insert errors.

## 2. Migration Scripts

All SQL migrations live under `migrations/postgres/`:

- `20250528000002_create_trades.sql`: Initial partitioned table and first few month partitions.
- `20250528000001_partition_orders.sql`: Initial partitioned orders table.
- `20250701000001_partition_trades.sql`: Rolling migration to parent table and dynamic partition creation for `trades`.
- `20250701000002_partition_orders_dynamic.sql`: Dynamic creation of monthly partitions for `orders` and default partition.
- `20250701000003_partition_trades_yearly.sql`: Create yearly partitions for `trades` (current ±1 year).
- `20250701000004_partition_orders_yearly.sql`: Create yearly partitions for `orders` (current ±1 year).

To apply migrations, run your standard migration tool (e.g., `psql` or `migrate`):
```sh
psql -h HOST -U USER -d DATABASE -f migrations/postgres/20250701000001_partition_trades.sql
psql -h HOST -U USER -d DATABASE -f migrations/postgres/20250701000002_partition_orders_dynamic.sql
```

## 3. Query Efficiency and ACID Compliance

- **Partition Pruning**: Ensure all queries against `orders` and `trades` include a `WHERE created_at BETWEEN ...` clause to enable PostgreSQL to prune unnecessary partitions.
- **Indexes**: Define indexes on the parent tables (applies to all partitions):
  - `idx_trades_user_created (user_id, created_at DESC)`
  - `idx_trades_symbol_side (symbol, side)`
  - `idx_orders_user_created (user_id, created_at DESC)`
  - `idx_orders_symbol_status (symbol, status)`
- **ACID Semantics**: Partitions are fully transparent; transactions spanning multiple partitions maintain ACID guarantees.

## 4. Backup and Restore Strategies

- **Full Dump**: Use `pg_dump` on the entire database periodically.
  ```sh
  pg_dump -h HOST -U USER -Fc DATABASE > backup_$(date +%F).dump
  ```
- **Partition-Level Backup**: For large data volumes, you can back up individual partitions:
  ```sh
  pg_dump -h HOST -U USER -t public.trades_2025_06 -Fc DATABASE > trades_2025_06.dump
  ```
- **Restore**:
  ```sh
  pg_restore -h HOST -U USER -d DATABASE backup_2025-05.dump
  ```

## 5. Automated Maintenance

- **Scheduled Partition Jobs**: Set up a `cron` or a database scheduler (`pg_cron`) to run the dynamic partition scripts monthly.
- **Old Partition Cleanup**: Optionally detach and archive or drop partitions older than a retention period:
  ```sql
  ALTER TABLE public.trades DETACH PARTITION trades_2023_01;
  DROP TABLE IF EXISTS trades_2023_01;
  ```
- **Monitoring**: Track partition sizes via `pg_partition_size()` and alert if partitions grow beyond expected size.
- **Retention Policy (6 Months)**: Keep only the last 6 monthly partitions online. Use archive scripts (below) to detach and archive older partitions.
- **Archiving Script**: A maintenance SQL script `migrations/postgres/partition_archiving.sql` detaches partitions older than 6 months and moves them to historical schema or drops them after backup.

## 6. Future Partition Creation

1. Verify the `migrations/postgres/20250701000001_partition_trades.sql` and `20250701000002_partition_orders_dynamic.sql` scripts include the correct date ranges.
2. If manual partition creation is needed, use the format:
   ```sql
   CREATE TABLE public.trades_YYYY_MM PARTITION OF public.trades
     FOR VALUES FROM ('YYYY-MM-01') TO ('YYYY-MM-01'::date + INTERVAL '1 month');
   ```
3. Commit a new timestamped migration under `migrations/postgres/`.

---
*Maintained by the Infrastructure Team. Last updated: 2025-07-01*
