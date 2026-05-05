package csvio

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sxwebdev/tron-balance-checker/internal/store"
)

// WriteRows writes the given rows to a CSV file with the header
// "address,trx,usdt,is_activated,checked_at,status". The file is created or
// overwritten.
func WriteRows(path string, rows []store.Row) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create output csv: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)

	if err := w.Write([]string{"address", "trx", "usdt", "is_activated", "checked_at", "status"}); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	for _, r := range rows {
		var checkedAt string
		if r.CheckedAt.Valid {
			checkedAt = r.CheckedAt.Time.UTC().Format(time.RFC3339)
		}
		if err := w.Write([]string{
			r.Address,
			r.TrxBalance,
			r.UsdtBalance,
			strconv.FormatBool(r.IsActivated),
			checkedAt,
			r.Status,
		}); err != nil {
			return fmt.Errorf("write row %q: %w", r.Address, err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}
	return nil
}
