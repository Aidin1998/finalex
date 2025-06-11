-- Migration: Add batch operations indexes
CREATE INDEX accounts_id_idx ON accounts (id);
CREATE INDEX orders_user_symbol_idx ON orders (user_id, symbol);
CREATE INDEX orders_user_status_idx ON orders (user_id, status);
CREATE INDEX orders_symbol_status_price_idx ON orders (symbol, status, price);
CREATE INDEX orders_batch_ids_idx ON orders (id) WHERE status IN ('pending', 'partially_filled');
CREATE INDEX orders_symbol_side_price_covering_idx ON orders (symbol, side, price);
