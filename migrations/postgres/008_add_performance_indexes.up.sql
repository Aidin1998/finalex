-- Migration: Add performance indexes
CREATE INDEX idx_orders_user_created_status ON orders (user_id, created_at DESC, status);
CREATE INDEX idx_orders_symbol_status_price ON orders (symbol, status, price) WHERE status IN ('active', 'partial');
CREATE INDEX idx_orders_symbol_side_price_quantity ON orders (symbol, side, price, quantity) WHERE status = 'active';
CREATE INDEX idx_orders_symbol_side_status_created ON orders (symbol, side, status, created_at DESC) WHERE status IN ('active', 'partial');
CREATE INDEX idx_trades_user_created_symbol ON trades (user_id, created_at DESC, symbol);
CREATE INDEX idx_trades_symbol_created_side ON trades (symbol, created_at DESC, side);
CREATE INDEX idx_trades_order_id ON trades (order_id);
CREATE INDEX idx_trades_counter_order_id ON trades (counter_order_id);
CREATE INDEX idx_trades_symbol_created_quantity ON trades (symbol, created_at DESC, quantity);
