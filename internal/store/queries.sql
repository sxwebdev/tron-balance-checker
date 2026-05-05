-- name: InsertAddress :execrows
INSERT INTO addresses (address) VALUES (?)
ON CONFLICT(address) DO NOTHING;

-- name: CountPending :one
SELECT COUNT(*) FROM addresses WHERE status = 'pending';

-- name: CountByStatus :one
SELECT COUNT(*) FROM addresses WHERE status = ?;

-- name: FetchPendingBatch :many
SELECT address FROM addresses WHERE status = 'pending' LIMIT ?;

-- name: MarkChecked :exec
UPDATE addresses
SET trx_balance = ?,
    usdt_balance = ?,
    status = 'ok',
    attempts = attempts + 1,
    checked_at = CURRENT_TIMESTAMP,
    error = ''
WHERE address = ?;

-- name: MarkFailed :exec
UPDATE addresses
SET status = 'failed',
    attempts = attempts + 1,
    checked_at = CURRENT_TIMESTAMP,
    error = ?
WHERE address = ?;

-- name: ResetAll :exec
UPDATE addresses
SET status = 'pending',
    attempts = 0,
    error = '',
    checked_at = NULL;

-- name: ListAll :many
SELECT address, trx_balance, usdt_balance, status, checked_at
FROM addresses
ORDER BY CAST(trx_balance AS REAL) DESC, address ASC;
