package tron

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/sxwebdev/gotron/pkg/client"

	"github.com/sxwebdev/tron-balance-checker/internal/config"
)

type Client struct {
	c        *client.Client
	usdtAddr string
	usdtDec  int32
}

func New(cfg *config.Config) (*Client, error) {
	if len(cfg.Nodes) == 0 {
		return nil, errors.New("tron: no nodes configured")
	}

	nodes := make([]client.NodeConfig, 0, len(cfg.Nodes))
	for _, n := range cfg.Nodes {
		nc := client.NodeConfig{
			Protocol: client.ProtocolGRPC,
			Address:  n.GrpcAddr,
			UseTLS:   n.UseTLS,
		}
		if h := parseHeaders(n.Headers); len(h) > 0 {
			nc.Headers = h
		}
		nodes = append(nodes, nc)
	}

	c, err := client.New(client.Config{
		Nodes:      nodes,
		Network:    client.NetworkMainnet,
		Blockchain: "tron",
	})
	if err != nil {
		return nil, fmt.Errorf("tron: create client: %w", err)
	}

	return &Client{
		c:        c,
		usdtAddr: cfg.USDT.Contract,
		usdtDec:  cfg.USDT.Decimals,
	}, nil
}

func (c *Client) Close() error { return c.c.Close() }

// GetTRX returns the TRX balance of an address. A non-activated account
// (one that never received any transaction) is reported as zero balance.
func (c *Client) GetTRX(ctx context.Context, addr string) (decimal.Decimal, error) {
	bal, err := c.c.GetAccountBalance(ctx, addr)
	if err != nil {
		if errors.Is(err, client.ErrAccountNotFound) {
			return decimal.Zero, nil
		}
		return decimal.Zero, fmt.Errorf("get trx balance: %w", err)
	}
	return bal, nil
}

// GetUSDT returns the USDT (TRC20) balance of an address scaled by the
// configured decimals.
func (c *Client) GetUSDT(ctx context.Context, addr string) (decimal.Decimal, error) {
	raw, err := c.c.TRC20ContractBalance(ctx, addr, c.usdtAddr)
	if err != nil {
		return decimal.Zero, fmt.Errorf("get usdt balance: %w", err)
	}
	if raw == nil {
		return decimal.Zero, nil
	}
	return decimal.NewFromBigInt(raw, -c.usdtDec), nil
}

// parseHeaders accepts "key1=value1;key2=value2" form and returns a metadata map.
// Returns nil for an empty input.
func parseHeaders(s string) map[string]string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	out := make(map[string]string)
	for part := range strings.SplitSeq(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
