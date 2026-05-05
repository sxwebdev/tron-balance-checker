package csvio

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sxwebdev/gotron/pkg/address"
)

// LoadAndValidate reads a single-column CSV without a header, deduplicates
// entries, and validates every address. If any address is invalid, every
// invalid entry is reported (with its CSV line number) and a non-nil error is
// returned. The returned slice preserves the order of first occurrence.
func LoadAndValidate(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	var (
		out      []string
		seen     = make(map[string]struct{})
		errs     []error
		duplicat int
		lineNo   int
	)

	for {
		lineNo++
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read csv line %d: %w", lineNo, err)
		}
		if len(rec) == 0 {
			continue
		}
		addr := strings.TrimSpace(rec[0])
		if addr == "" {
			continue
		}
		if _, ok := seen[addr]; ok {
			duplicat++
			continue
		}
		seen[addr] = struct{}{}
		if err := address.Validate(addr); err != nil {
			errs = append(errs, fmt.Errorf("line %d: %q is not a valid Tron address: %w", lineNo, addr, err))
			continue
		}
		out = append(out, addr)
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("found %d invalid address(es):\n%w", len(errs), errors.Join(errs...))
	}
	if len(out) == 0 {
		return nil, errors.New("csv contains no addresses")
	}
	return out, nil
}
