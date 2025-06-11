-- Down migration for performance indexes (removes only the indexes created in the up migration)
DROP INDEX IF EXISTS idx_orders_user_created_status;
DROP INDEX IF EXISTS idx_orders_symbol_status_price;
DROP INDEX IF EXISTS idx_orders_symbol_side_price_quantity;
DROP INDEX IF EXISTS idx_orders_symbol_side_status_created;
DROP INDEX IF EXISTS idx_trades_user_created_symbol;
DROP INDEX IF EXISTS idx_trades_symbol_created_side;
DROP INDEX IF EXISTS idx_trades_order_id;
DROP INDEX IF EXISTS idx_trades_counter_order_id;
DROP INDEX IF EXISTS idx_trades_symbol_created_quantity;
