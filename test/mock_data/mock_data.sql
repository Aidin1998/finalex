-- Mock data for accounts table
INSERT INTO accounts (id, email, username, password_hash, first_name, last_name, phone, date_of_birth, country, status, kyc_level, tier, mfa_enabled, email_verified, phone_verified, last_login, created_at, updated_at, preferences, roles)
VALUES
  (gen_random_uuid(), 'alice@example.com', 'alice', 'hashed_pw1', 'Alice', 'Smith', '+1234567890', '1990-01-01', 'US', 'active', 2, 'premium', true, true, true, NOW(), NOW(), NOW(), '{"theme":"dark"}', '["user"]'),
  (gen_random_uuid(), 'bob@example.com', 'bob', 'hashed_pw2', 'Bob', 'Johnson', '+1987654321', '1985-05-15', 'GB', 'pending_verification', 1, 'basic', false, false, false, NULL, NOW(), NOW(), '{"theme":"light"}', '["user"]'),
  (gen_random_uuid(), 'carol@example.com', 'carol', 'hashed_pw3', 'Carol', 'Williams', '+1122334455', '1978-09-23', 'CA', 'suspended', 3, 'vip', true, true, false, NOW(), NOW(), NOW(), '{"theme":"auto"}', '["admin"]');

-- Mock data for reservations table
INSERT INTO reservations (id, user_id, currency, amount, type, reference_id, status, expires_at, version, created_at, updated_at)
SELECT gen_random_uuid(), id, 'USD', 100.00, 'order', 'ref-1', 'active', NOW() + INTERVAL '1 day', 1, NOW(), NOW() FROM accounts LIMIT 1;
INSERT INTO reservations (id, user_id, currency, amount, type, reference_id, status, expires_at, version, created_at, updated_at)
SELECT gen_random_uuid(), id, 'BTC', 0.5, 'withdrawal', 'ref-2', 'released', NOW() + INTERVAL '2 days', 1, NOW(), NOW() FROM accounts LIMIT 1 OFFSET 1;

-- Mock data for transaction_journal table
INSERT INTO transaction_journal (id, user_id, currency, type, amount, balance_before, balance_after, available_before, available_after, locked_before, locked_after, reference_id, description, metadata, status, created_at)
SELECT gen_random_uuid(), id, 'USD', 'deposit', 1000.00, 0, 1000.00, 0, 1000.00, 0, 0, 'ref-1', 'Initial deposit', '{"source":"bank"}', 'completed', NOW() FROM accounts LIMIT 1;
INSERT INTO transaction_journal (id, user_id, currency, type, amount, balance_before, balance_after, available_before, available_after, locked_before, locked_after, reference_id, description, metadata, status, created_at)
SELECT gen_random_uuid(), id, 'BTC', 'trade_buy', 0.1, 0, 0.1, 0, 0.1, 0, 0, 'ref-2', 'Bought BTC', '{"pair":"BTC/USD"}', 'completed', NOW() FROM accounts LIMIT 1 OFFSET 1;

-- Mock data for orders table
INSERT INTO orders (id, user_id, symbol, side, price, quantity, status, created_at, updated_at)
SELECT gen_random_uuid(), id, 'BTC/USD', 'buy', 30000.00, 0.05, 'pending', NOW(), NOW() FROM accounts LIMIT 1;
INSERT INTO orders (id, user_id, symbol, side, price, quantity, status, created_at, updated_at)
SELECT gen_random_uuid(), id, 'ETH/USD', 'sell', 2000.00, 1.2, 'completed', NOW(), NOW() FROM accounts LIMIT 1 OFFSET 1;

-- Mock data for trades table
INSERT INTO trades (id, order_id, counter_order_id, user_id, counter_user_id, symbol, side, price, quantity, fee, fee_currency, created_at)
SELECT gen_random_uuid(), o1.id, o2.id, o1.user_id, o2.user_id, 'BTC/USD', 'buy', 30000.00, 0.05, 1.5, 'USD', NOW()
FROM orders o1, orders o2 WHERE o1.id <> o2.id LIMIT 1;

-- Mock data for compliance_alerts and aml_users
INSERT INTO compliance_alerts (id, user_id, type, severity, message, status, assigned_to, notes, metadata, created_at, updated_at)
VALUES (gen_random_uuid(), 'alice', 'withdrawal', 'high', 'Large withdrawal flagged', 'open', 'admin', 'Check source of funds', '{"risk":"high"}', NOW(), NOW());
INSERT INTO aml_users (id, user_id, risk_level, risk_score, kyc_status, is_blacklisted, is_whitelisted, last_risk_update, country_code, is_high_risk_country, pep_status, sanction_status, customer_type, business_type, risk_factors, created_at, updated_at)
SELECT gen_random_uuid(), id, 'HIGH', 85.5, 'approved', false, true, NOW(), 'US', false, false, false, 'Individual', NULL, '{"source":"crypto"}', NOW(), NOW() FROM accounts LIMIT 1;
