package checker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/sxwebdev/xutils/loopper"
	"github.com/sxwebdev/xutils/retry"
	"golang.org/x/time/rate"

	"github.com/sxwebdev/tron-balance-checker/internal/config"
	"github.com/sxwebdev/tron-balance-checker/internal/store"
	"github.com/sxwebdev/tron-balance-checker/internal/tron"
)

type Checker struct {
	cfg   *config.Config
	store *store.Store
	tron  *tron.Client
	log   *slog.Logger
}

func New(cfg *config.Config, st *store.Store, tc *tron.Client, log *slog.Logger) *Checker {
	return &Checker{cfg: cfg, store: st, tron: tc, log: log}
}

// Run drains the pending queue. It returns once every address has been moved
// out of `pending` (to either `ok` or `failed`) or ctx is cancelled.
func (c *Checker) Run(ctx context.Context) error {
	total, err := c.store.CountPending(ctx)
	if err != nil {
		return fmt.Errorf("count pending: %w", err)
	}
	if total == 0 {
		c.log.Info("nothing to check")
		return nil
	}

	startedAt := time.Now()
	c.log.Info("starting balance check",
		slog.Int64("pending", total),
		slog.Int("rate_limit_rps", c.cfg.Checker.RateLimit),
		slog.Int("batch_size", c.cfg.Checker.BatchSize),
		slog.Int("nodes", len(c.cfg.Nodes)),
	)

	bar := progressbar.NewOptions64(total,
		progressbar.OptionSetDescription("checking"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("addr"),
		progressbar.OptionThrottle(100_000_000), // 100ms in ns
		progressbar.OptionClearOnFinish(),
	)

	limiter := rate.NewLimiter(rate.Limit(c.cfg.Checker.RateLimit), 1)

	var (
		ok     atomic.Int64
		failed atomic.Int64
		done   = make(chan struct{})
		once   sync.Once
	)

	finish := func() { once.Do(func() { close(done) }) }

	work := func(loopCtx context.Context) {
		// Use the parent ctx for DB/network so per-iteration timeouts from
		// loopper don't kill in-flight operations prematurely.
		batch, err := c.store.FetchPendingBatch(ctx, c.cfg.Checker.BatchSize)
		if err != nil {
			c.log.Error("fetch pending batch", slog.String("err", err.Error()))
			return
		}
		if len(batch) == 0 {
			finish()
			return
		}

		for _, addr := range batch {
			if ctx.Err() != nil {
				return
			}
			if err := limiter.Wait(ctx); err != nil {
				return
			}

			err := retry.New(
				retry.WithMaxAttempts(c.cfg.Checker.RetryMax),
				retry.WithPolicy(retry.PolicyBackoff),
				retry.WithDelay(c.cfg.Checker.RetryDelay),
			).Do(func() error {
				if ctx.Err() != nil {
					return retry.ErrExit
				}
				return c.checkOne(ctx, addr)
			})

			if err != nil && !errors.Is(err, retry.ErrExit) {
				if mErr := c.store.MarkFailed(ctx, addr, err.Error()); mErr != nil {
					c.log.Error("mark failed",
						slog.String("address", addr),
						slog.String("err", mErr.Error()))
				}
				failed.Add(1)
				c.log.Warn("address check failed",
					slog.String("address", addr),
					slog.String("err", err.Error()))
			} else if err == nil {
				ok.Add(1)
			}
			_ = bar.Add(1)
		}
	}

	l := loopper.New(work,
		loopper.WithPeriod(c.cfg.Checker.LoopPeriod),
		loopper.WithContextTimeout(0), // disable per-tick timeout; we manage timeouts on individual RPCs
		loopper.WithLeading(),
	)
	l.Start(ctx)

	select {
	case <-done:
	case <-ctx.Done():
	}
	l.Stop()
	l.Wait()
	_ = bar.Finish()

	c.log.Info("check finished",
		slog.Int64("ok", ok.Load()),
		slog.Int64("failed", failed.Load()),
		slog.Int64("total", total),
		slog.Duration("elapsed", time.Since(startedAt).Round(time.Millisecond)),
	)

	if pending, err := c.store.CountPending(ctx); err == nil && pending > 0 && ctx.Err() == nil {
		return fmt.Errorf("checker stopped with %d addresses still pending", pending)
	}
	return ctx.Err()
}

func (c *Checker) checkOne(ctx context.Context, addr string) error {
	rctx, cancel := context.WithTimeout(ctx, c.cfg.Checker.RequestTimeout)
	defer cancel()

	trxBal, err := c.tron.GetTRX(rctx, addr)
	if err != nil {
		return fmt.Errorf("trx: %w", err)
	}

	// A positive TRX balance implies the account has been activated on chain.
	// Only fall back to a dedicated activation lookup when the balance is zero.
	activated := trxBal.IsPositive()
	if !activated {
		activated, err = c.tron.IsActivated(rctx, addr)
		if err != nil {
			return fmt.Errorf("activated: %w", err)
		}
	}

	usdtBal, err := c.tron.GetUSDT(rctx, addr)
	if err != nil {
		return fmt.Errorf("usdt: %w", err)
	}
	if err := c.store.MarkChecked(ctx, addr, trxBal.String(), usdtBal.String(), activated); err != nil {
		return fmt.Errorf("mark checked: %w", err)
	}
	return nil
}
