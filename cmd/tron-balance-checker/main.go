package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"

	"github.com/sxwebdev/tron-balance-checker/internal/checker"
	"github.com/sxwebdev/tron-balance-checker/internal/config"
	"github.com/sxwebdev/tron-balance-checker/internal/csvio"
	"github.com/sxwebdev/tron-balance-checker/internal/store"
	"github.com/sxwebdev/tron-balance-checker/internal/tron"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app := &cli.Command{
		Name:  "tron-balance-checker",
		Usage: "Check TRX and USDT (TRC20) balances of Tron addresses in batches",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   "configs/config.yaml",
				Usage:   "Path to YAML config file",
				Sources: cli.EnvVars("TBC_CONFIG"),
			},
		},
		Commands: []*cli.Command{
			importCommand(logger),
			checkCommand(logger),
			exportCommand(logger),
		},
	}

	if err := app.Run(ctx, os.Args); err != nil {
		logger.Error("command failed", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func importCommand(log *slog.Logger) *cli.Command {
	return &cli.Command{
		Name:  "import",
		Usage: "Validate addresses from a CSV file and store them in SQLite",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "file",
				Aliases:  []string{"f"},
				Usage:    "Path to input CSV (one address per line, no header)",
				Required: true,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := config.Load(cmd.String("config"))
			if err != nil {
				return err
			}

			addrs, err := csvio.LoadAndValidate(cmd.String("file"))
			if err != nil {
				return err
			}
			log.Info("csv parsed", slog.Int("valid_addresses", len(addrs)))

			st, err := store.New(ctx, cfg.Database.Path)
			if err != nil {
				return err
			}
			defer st.Close()

			added, skipped, err := st.InsertAddresses(ctx, addrs)
			if err != nil {
				return err
			}
			log.Info("addresses imported",
				slog.Int("added", added),
				slog.Int("already_present", skipped),
				slog.String("db", cfg.Database.Path),
			)
			return nil
		},
	}
}

func checkCommand(log *slog.Logger) *cli.Command {
	return &cli.Command{
		Name:  "check",
		Usage: "Fetch TRX and USDT balances for all pending addresses",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "recheck",
				Usage: "Reset all addresses to pending and re-check them",
				Value: false,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := config.Load(cmd.String("config"))
			if err != nil {
				return err
			}

			st, err := store.New(ctx, cfg.Database.Path)
			if err != nil {
				return err
			}
			defer st.Close()

			if cmd.Bool("recheck") {
				if err := st.ResetAll(ctx); err != nil {
					return fmt.Errorf("reset addresses: %w", err)
				}
				log.Info("all addresses reset to pending")
			}

			tc, err := tron.New(cfg)
			if err != nil {
				return err
			}
			defer tc.Close()

			ch := checker.New(cfg, st, tc, log)
			if err := ch.Run(ctx); err != nil {
				if errors.Is(err, context.Canceled) {
					log.Warn("interrupted, exiting")
					return nil
				}
				return err
			}
			return nil
		},
	}
}

func exportCommand(log *slog.Logger) *cli.Command {
	return &cli.Command{
		Name:  "export",
		Usage: "Export all addresses with balances to a CSV sorted by TRX desc",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Value:   "output.csv",
				Usage:   "Path to output CSV file",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := config.Load(cmd.String("config"))
			if err != nil {
				return err
			}

			st, err := store.New(ctx, cfg.Database.Path)
			if err != nil {
				return err
			}
			defer st.Close()

			rows, err := st.ListAll(ctx)
			if err != nil {
				return err
			}

			outPath := cmd.String("output")
			if err := csvio.WriteRows(outPath, rows); err != nil {
				return err
			}
			log.Info("export written",
				slog.Int("rows", len(rows)),
				slog.String("path", outPath),
			)
			return nil
		},
	}
}
