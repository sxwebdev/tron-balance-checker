# tron-balance-checker

A CLI tool that bulk-checks TRX and USDT (TRC20) balances for a list of Tron
addresses. Addresses are taken from a CSV, validated, stored in a local
SQLite database, and then walked through with a configurable rate limit and
round-robin across multiple nodes via
[gotron](https://github.com/sxwebdev/gotron). The result is exported to a CSV
sorted by TRX balance from largest to smallest.

## Installation

```bash
go install github.com/sxwebdev/tron-balance-checker/cmd/tron-balance-checker@latest
```

or build from source:

```bash
git clone https://github.com/sxwebdev/tron-balance-checker.git
cd tron-balance-checker
go build -o tron-balance-checker ./cmd/tron-balance-checker
```

## Configuration

The config lives in `configs/config.yaml` (override the path with `--config`).
Full example:

```yaml
nodes:
  - grpc_addr: tron-grpc.publicnode.com:443
    headers: ""              # optional, format "key1=value1;key2=value2"
    use_tls: true

usdt:
  contract: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"   # USDT contract on mainnet
  decimals: 6

checker:
  rate_limit: 3              # requests per second (shared across all nodes)
  batch_size: 50             # how many addresses to pull from the DB per tick
  retry_max: 3               # how many times to retry on error
  retry_delay: 1s            # initial retry delay (exponential backoff)
  request_timeout: 15s       # timeout per RPC call
  loop_period: 100ms         # delay between batches

database:
  path: data/sqlite/db.sqlite
```

Multiple nodes can be listed — gotron rotates between them in round-robin
fashion:

```yaml
nodes:
  - grpc_addr: tron-grpc.publicnode.com:443
    use_tls: true
  - grpc_addr: grpc.trongrid.io:50051
    headers: "TRON-PRO-API-KEY=your-api-key"
    use_tls: false
```

Any field under `checker:` or `database:` can also be overridden via environment
variables prefixed with `TBC_` (e.g. `TBC_RATE_LIMIT=5`).

## Usage

The full pipeline: import → check → export.

### 1. Import addresses

The input CSV is one column without a header — each row is a Tron address:

```text
TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t
TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7
TKzxdSv2FZKQrEqkKVgp5DcwEXBEKMg2Ax
```

```bash
./tron-balance-checker import --file addresses.csv
```

If at least one address fails validation (base58 format + checksum + `0x41`
prefix), the command prints every error with line numbers and exits with code
`1`; **nothing is written to the DB**. Duplicates within the CSV are silently
collapsed. Addresses already present in the DB are skipped.

### 2. Check balances

```bash
./tron-balance-checker check
```

A progress bar is rendered to stderr; structured logs go to stdout. The
command does not return until every `pending` address has been processed. On
failure each address is retried `retry_max` times with exponential backoff; if
all attempts are exhausted the address is marked `status=failed` and the error
text is stored in the DB.

Re-check everything (including already `ok` and `failed`):

```bash
./tron-balance-checker check --recheck
```

`Ctrl+C` stops the checker gracefully — the in-flight operation is committed,
progress is preserved, and the next run continues from the same point.

### 3. Export results

```bash
./tron-balance-checker export --output output.csv
```

Output format:

```csv
address,trx,usdt,checked_at,status
TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t,123.456789,4567.890123,2026-05-05T11:30:14Z,ok
TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7,12.5,0,2026-05-05T11:30:15Z,ok
...
```

Sorted by `trx` desc. The `status` column is one of `ok`, `failed`, or
`pending`.

## Storage

The SQLite file is created automatically at the path from the config (default
`data/sqlite/db.sqlite`). Single-table schema:

| Column         | Type      | Description                                 |
| -------------- | --------- | ------------------------------------------- |
| `address`      | TEXT PK   | Tron address                                |
| `trx_balance`  | TEXT      | TRX balance as a decimal string             |
| `usdt_balance` | TEXT      | USDT balance as a decimal string            |
| `status`       | TEXT      | `pending` \| `ok` \| `failed`               |
| `error`        | TEXT      | Last error message (for `failed`)           |
| `attempts`     | INTEGER   | Attempt counter                             |
| `checked_at`   | TIMESTAMP | Time of the last attempt                    |
| `created_at`   | TIMESTAMP | Time the address was added to the DB        |

## Development

SQL queries live in [internal/store/queries.sql](internal/store/queries.sql)
and are code-generated with [sqlc](https://github.com/sqlc-dev/sqlc). After
editing queries or schema:

```bash
sqlc generate
```

Generated code in [internal/store/db/](internal/store/db/) is committed
alongside the sources.

## License

[MIT](LICENSE)
