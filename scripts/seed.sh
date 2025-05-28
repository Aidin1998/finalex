#!/bin/bash

# Seed script for Pincex

set -e

echo "Seeding database with initial data..."

# Check if PINCEX_DATABASE_DSN is set
if [ -z "$PINCEX_DATABASE_DSN" ]; then
    # Try to load from .env file
    if [ -f .env ]; then
        export $(grep -v '^#' .env | xargs)
    else
        echo "Error: PINCEX_DATABASE_DSN environment variable not set and .env file not found"
        exit 1
    fi
fi

# Seed trading pairs
echo "Seeding trading pairs..."
psql "$PINCEX_DATABASE_DSN" <<EOF
INSERT INTO trading_pairs (id, symbol, base_currency, quote_currency, price_precision, amount_precision, min_order_size, max_order_size, min_price, max_price, maker_fee, taker_fee, status, created_at, updated_at)
VALUES
    ('$(uuidgen)', 'BTC-USDT', 'BTC', 'USDT', 2, 6, 0.0001, 1000, 1, 1000000, 0.001, 0.002, 'active', NOW(), NOW()),
    ('$(uuidgen)', 'ETH-USDT', 'ETH', 'USDT', 2, 5, 0.001, 5000, 1, 100000, 0.001, 0.002, 'active', NOW(), NOW()),
    ('$(uuidgen)', 'BNB-USDT', 'BNB', 'USDT', 2, 4, 0.01, 10000, 1, 10000, 0.001, 0.002, 'active', NOW(), NOW()),
    ('$(uuidgen)', 'SOL-USDT', 'SOL', 'USDT', 3, 2, 0.1, 50000, 0.1, 10000, 0.001, 0.002, 'active', NOW(), NOW()),
    ('$(uuidgen)', 'XRP-USDT', 'XRP', 'USDT', 5, 1, 1, 1000000, 0.0001, 1000, 0.001, 0.002, 'active', NOW(), NOW()),
    ('$(uuidgen)', 'ADA-USDT', 'ADA', 'USDT', 5, 1, 1, 1000000, 0.0001, 1000, 0.001, 0.002, 'active', NOW(), NOW()),
    ('$(uuidgen)', 'DOGE-USDT', 'DOGE', 'USDT', 6, 0, 10, 10000000, 0.00001, 10, 0.001, 0.002, 'active', NOW(), NOW()),
    ('$(uuidgen)', 'DOT-USDT', 'DOT', 'USDT', 4, 2, 0.1, 100000, 0.01, 1000, 0.001, 0.002, 'active', NOW(), NOW()),
    ('$(uuidgen)', 'MATIC-USDT', 'MATIC', 'USDT', 5, 1, 1, 1000000, 0.0001, 100, 0.001, 0.002, 'active', NOW(), NOW()),
    ('$(uuidgen)', 'AVAX-USDT', 'AVAX', 'USDT', 3, 2, 0.1, 50000, 0.1, 1000, 0.001, 0.002, 'active', NOW(), NOW())
ON CONFLICT (symbol) DO NOTHING;
EOF

# Seed market prices
echo "Seeding market prices..."
psql "$PINCEX_DATABASE_DSN" <<EOF
INSERT INTO market_prices (symbol, price, change_24h, volume_24h, high_24h, low_24h, updated_at)
VALUES
    ('BTC-USDT', 65000.00, 2.5, 1250000000, 66000.00, 64000.00, NOW()),
    ('ETH-USDT', 3500.00, 1.8, 750000000, 3600.00, 3400.00, NOW()),
    ('BNB-USDT', 450.00, 0.5, 250000000, 455.00, 445.00, NOW()),
    ('SOL-USDT', 120.00, 3.2, 180000000, 125.00, 115.00, NOW()),
    ('XRP-USDT', 0.55000, -0.8, 120000000, 0.56000, 0.54000, NOW()),
    ('ADA-USDT', 0.45000, 1.2, 90000000, 0.46000, 0.44000, NOW()),
    ('DOGE-USDT', 0.080000, 5.5, 85000000, 0.085000, 0.075000, NOW()),
    ('DOT-USDT', 7.5000, -1.5, 65000000, 7.7000, 7.3000, NOW()),
    ('MATIC-USDT', 0.85000, 2.1, 55000000, 0.87000, 0.83000, NOW()),
    ('AVAX-USDT', 28.000, 4.3, 45000000, 29.000, 27.000, NOW())
ON CONFLICT (symbol) DO UPDATE SET
    price = EXCLUDED.price,
    change_24h = EXCLUDED.change_24h,
    volume_24h = EXCLUDED.volume_24h,
    high_24h = EXCLUDED.high_24h,
    low_24h = EXCLUDED.low_24h,
    updated_at = EXCLUDED.updated_at;
EOF

# Create admin user
echo "Creating admin user..."
ADMIN_UUID=$(uuidgen)
ADMIN_PASSWORD_HASH=$(echo -n "admin123" | sha256sum | awk '{print $1}')
psql "$PINCEX_DATABASE_DSN" <<EOF
INSERT INTO users (id, email, username, password_hash, first_name, last_name, kyc_status, two_fa_enabled, created_at, updated_at)
VALUES
    ('$ADMIN_UUID', 'admin@pincex.com', 'admin', '$ADMIN_PASSWORD_HASH', 'Admin', 'User', 'approved', false, NOW(), NOW())
ON CONFLICT (email) DO NOTHING;
EOF

# Create admin account
echo "Creating admin accounts..."
psql "$PINCEX_DATABASE_DSN" <<EOF
INSERT INTO accounts (id, user_id, currency, balance, available, locked, created_at, updated_at)
VALUES
    ('$(uuidgen)', '$ADMIN_UUID', 'BTC', 10.0, 10.0, 0.0, NOW(), NOW()),
    ('$(uuidgen)', '$ADMIN_UUID', 'ETH', 100.0, 100.0, 0.0, NOW(), NOW()),
    ('$(uuidgen)', '$ADMIN_UUID', 'USDT', 1000000.0, 1000000.0, 0.0, NOW(), NOW())
ON CONFLICT (user_id, currency) DO NOTHING;
EOF

echo "Database seeding completed successfully!"
