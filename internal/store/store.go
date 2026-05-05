package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/sxwebdev/tron-balance-checker/internal/store/db"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
	db *sql.DB
	q  *db.Queries
}

func New(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)"
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if _, err := sqldb.ExecContext(ctx, schemaSQL); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	if err := migrate(ctx, sqldb); err != nil {
		_ = sqldb.Close()
		return nil, err
	}

	return &Store{db: sqldb, q: db.New(sqldb)}, nil
}

// migrate brings older databases up to the current schema. SQLite has no
// "ALTER TABLE ... ADD COLUMN IF NOT EXISTS", so we inspect pragma_table_info
// and only add columns that are missing.
func migrate(ctx context.Context, sqldb *sql.DB) error {
	for _, m := range []struct {
		column string
		ddl    string
	}{
		{"is_activated", "ALTER TABLE addresses ADD COLUMN is_activated BOOLEAN NOT NULL DEFAULT 0"},
	} {
		var n int
		if err := sqldb.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM pragma_table_info('addresses') WHERE name = ?",
			m.column,
		).Scan(&n); err != nil {
			return fmt.Errorf("inspect column %q: %w", m.column, err)
		}
		if n > 0 {
			continue
		}
		if _, err := sqldb.ExecContext(ctx, m.ddl); err != nil {
			return fmt.Errorf("add column %q: %w", m.column, err)
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

// InsertAddresses inserts addresses in a single transaction using INSERT OR IGNORE
// semantics. Returns counts of newly added rows and rows that already existed.
func (s *Store) InsertAddresses(ctx context.Context, addrs []string) (added, skipped int, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.q.WithTx(tx)
	for _, a := range addrs {
		n, err := qtx.InsertAddress(ctx, a)
		if err != nil {
			return 0, 0, fmt.Errorf("insert %q: %w", a, err)
		}
		if n > 0 {
			added++
		} else {
			skipped++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit tx: %w", err)
	}
	return added, skipped, nil
}

func (s *Store) CountPending(ctx context.Context) (int64, error) {
	return s.q.CountPending(ctx)
}

func (s *Store) CountByStatus(ctx context.Context, status string) (int64, error) {
	return s.q.CountByStatus(ctx, status)
}

func (s *Store) FetchPendingBatch(ctx context.Context, limit int) ([]string, error) {
	return s.q.FetchPendingBatch(ctx, int64(limit))
}

func (s *Store) MarkChecked(ctx context.Context, addr, trx, usdt string, activated bool) error {
	return s.q.MarkChecked(ctx, db.MarkCheckedParams{
		Address:     addr,
		TrxBalance:  trx,
		UsdtBalance: usdt,
		IsActivated: activated,
	})
}

func (s *Store) MarkFailed(ctx context.Context, addr, errMsg string) error {
	return s.q.MarkFailed(ctx, db.MarkFailedParams{
		Address: addr,
		Error:   errMsg,
	})
}

func (s *Store) ResetAll(ctx context.Context) error {
	return s.q.ResetAll(ctx)
}

// Row is a flattened representation suitable for export.
type Row = db.ListAllRow

func (s *Store) ListAll(ctx context.Context) ([]Row, error) {
	return s.q.ListAll(ctx)
}
