-- Down migration for batch operations indexes (removes only the indexes created in the up migration)
DROP INDEX IF EXISTS accounts_user_currency_idx;
DROP INDEX IF EXISTS accounts_user_id_idx;
DROP INDEX IF EXISTS accounts_currency_idx;
DROP INDEX IF EXISTS accounts_user_currency_balance_covering_idx;
DROP INDEX IF EXISTS orders_user_symbol_idx;
DROP INDEX IF EXISTS orders_user_status_idx;
DROP INDEX IF EXISTS orders_symbol_status_price_idx;
DROP INDEX IF EXISTS orders_batch_ids_idx;
DROP INDEX IF EXISTS orders_symbol_side_price_covering_idx;
