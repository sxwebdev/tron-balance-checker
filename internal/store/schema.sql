CREATE TABLE IF NOT EXISTS addresses (
  address       TEXT PRIMARY KEY,
  trx_balance   TEXT NOT NULL DEFAULT '0',
  usdt_balance  TEXT NOT NULL DEFAULT '0',
  status        TEXT NOT NULL DEFAULT 'pending',
  error         TEXT NOT NULL DEFAULT '',
  attempts      INTEGER NOT NULL DEFAULT 0,
  checked_at    TIMESTAMP,
  created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_addresses_status ON addresses(status);
