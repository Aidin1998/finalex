-- Index optimization scripts for Pincex CEX database query optimization
-- These indexes are specifically designed to support batch operations and resolve N+1 query patterns

-- =============================================================================
-- ACCOUNTS TABLE INDEXES
-- =============================================================================

-- Primary composite index for BatchGetAccounts optimization
-- This index eliminates table scans for user+currency lookups
CREATE INDEX CONCURRENTLY IF NOT EXISTS accounts_user_currency_idx 
ON accounts (user_id, currency);

-- Support index for user-based queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS accounts_user_id_idx 
ON accounts (user_id);

-- Support index for currency-based queries (lower priority)
CREATE INDEX CONCURRENTLY IF NOT EXISTS accounts_currency_idx 
ON accounts (currency);

-- Covering index for balance operations (includes frequently accessed columns)
CREATE INDEX CONCURRENTLY IF NOT EXISTS accounts_user_currency_balance_covering_idx 
ON accounts (user_id, currency) 
INCLUDE (balance, available, locked, updated_at);

-- =============================================================================
-- ORDERS TABLE INDEXES
-- =============================================================================

-- Composite index for batch order retrieval by user and symbol
CREATE INDEX CONCURRENTLY IF NOT EXISTS orders_user_symbol_idx 
ON orders (user_id, symbol);

-- Index for batch order retrieval by user and status
CREATE INDEX CONCURRENTLY IF NOT EXISTS orders_user_status_idx 
ON orders (user_id, status);

-- Composite index for order book queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS orders_symbol_status_price_idx 
ON orders (symbol, status, price);

-- Index for batch order operations by order IDs
CREATE INDEX CONCURRENTLY IF NOT EXISTS orders_batch_ids_idx 
ON orders (id) 
WHERE status IN ('pending', 'partially_filled');

-- Covering index for trading engine queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS orders_symbol_side_price_covering_idx 
ON orders (symbol, side, price) 
INCLUDE (quantity, filled_quantity, status, created_at)
WHERE status IN ('pending', 'partially_filled');

-- =============================================================================
-- TRADES TABLE INDEXES
-- =============================================================================

-- Index for batch trade retrieval by order IDs
CREATE INDEX CONCURRENTLY IF NOT EXISTS trades_order_id_idx 
ON trades (order_id);

-- Composite index for user trade history
CREATE INDEX CONCURRENTLY IF NOT EXISTS trades_user_symbol_timestamp_idx 
ON trades (user_id, symbol, created_at DESC);

-- Index for batch trade operations
CREATE INDEX CONCURRENTLY IF NOT EXISTS trades_batch_orders_idx 
ON trades (order_id, created_at DESC);

-- =============================================================================
-- TRANSACTIONS TABLE INDEXES
-- =============================================================================

-- Composite index for user transaction history
CREATE INDEX CONCURRENTLY IF NOT EXISTS transactions_user_currency_type_idx 
ON transactions (user_id, currency, type);

-- Index for transaction status queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS transactions_status_created_idx 
ON transactions (status, created_at)
WHERE status IN ('pending', 'processing');

-- Index for reference-based lookups
CREATE INDEX CONCURRENTLY IF NOT EXISTS transactions_reference_idx 
ON transactions (reference)
WHERE reference IS NOT NULL;

-- =============================================================================
-- SPECIALIZED INDEXES FOR BATCH OPERATIONS
-- =============================================================================

-- Partial index for active accounts (frequently accessed)
CREATE INDEX CONCURRENTLY IF NOT EXISTS accounts_active_user_currency_idx 
ON accounts (user_id, currency, updated_at)
WHERE balance > 0 OR available > 0 OR locked > 0;

-- Partial index for orders in active states
CREATE INDEX CONCURRENTLY IF NOT EXISTS orders_active_user_symbol_idx 
ON orders (user_id, symbol, updated_at)
WHERE status IN ('pending', 'partially_filled');

-- Index for batch funds operations (lock/unlock)
CREATE INDEX CONCURRENTLY IF NOT EXISTS accounts_funds_operations_idx 
ON accounts (user_id, currency, available, locked)
WHERE available > 0 OR locked > 0;

-- =============================================================================
-- PERFORMANCE MONITORING INDEXES
-- =============================================================================

-- Index for performance monitoring and analytics
CREATE INDEX CONCURRENTLY IF NOT EXISTS orders_performance_monitoring_idx 
ON orders (created_at, symbol, status)
WHERE created_at >= CURRENT_DATE - INTERVAL '7 days';

-- Index for trade volume analysis
CREATE INDEX CONCURRENTLY IF NOT EXISTS trades_volume_analysis_idx 
ON trades (symbol, created_at, quantity)
WHERE created_at >= CURRENT_DATE - INTERVAL '1 day';

-- =============================================================================
-- STATISTICS UPDATE COMMANDS
-- =============================================================================

-- Update statistics for query planner optimization
ANALYZE accounts;
ANALYZE orders;
ANALYZE trades;
ANALYZE transactions;

-- =============================================================================
-- INDEX USAGE MONITORING QUERIES
-- =============================================================================

-- Query to monitor index usage (PostgreSQL specific)
/*
SELECT 
    schemaname,
    tablename,
    indexname,
    idx_scan,
    idx_tup_read,
    idx_tup_fetch
FROM pg_stat_user_indexes
WHERE schemaname = 'public'
    AND tablename IN ('accounts', 'orders', 'trades', 'transactions')
ORDER BY idx_scan DESC;
*/

-- Query to find unused indexes
/*
SELECT 
    schemaname,
    tablename,
    indexname,
    idx_scan,
    pg_size_pretty(pg_relation_size(indexrelid)) as size
FROM pg_stat_user_indexes
WHERE schemaname = 'public'
    AND idx_scan = 0
    AND indexrelid IS NOT NULL;
*/

-- =============================================================================
-- INDEX MAINTENANCE COMMANDS
-- =============================================================================

-- Reindex commands for maintenance (run during low traffic periods)
/*
REINDEX INDEX CONCURRENTLY accounts_user_currency_idx;
REINDEX INDEX CONCURRENTLY orders_user_symbol_idx;
REINDEX INDEX CONCURRENTLY trades_order_id_idx;
*/

-- =============================================================================
-- PERFORMANCE IMPACT ESTIMATES
-- =============================================================================

/*
Index Performance Impact Estimates:

1. accounts_user_currency_idx:
   - Expected improvement: 50-80% reduction in BatchGetAccounts query time
   - Memory usage: ~2-4MB for 1M accounts
   - Maintenance overhead: Low

2. orders_user_symbol_idx:
   - Expected improvement: 40-60% reduction in batch order retrieval
   - Memory usage: ~5-10MB for 1M orders
   - Maintenance overhead: Medium (due to order updates)

3. trades_order_id_idx:
   - Expected improvement: 60-80% reduction in trade lookup by order
   - Memory usage: ~3-6MB for 1M trades
   - Maintenance overhead: Medium

4. accounts_funds_operations_idx:
   - Expected improvement: 30-50% reduction in funds lock/unlock operations
   - Memory usage: ~2-3MB for 1M accounts
   - Maintenance overhead: High (frequent balance updates)

Total estimated memory usage for all indexes: ~20-40MB
Total estimated query performance improvement: 40-70% average
*/
