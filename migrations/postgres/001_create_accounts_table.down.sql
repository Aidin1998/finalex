-- Down migration for accounts table (removes only the accounts table and related indexes)
DROP TABLE IF EXISTS accounts CASCADE;
