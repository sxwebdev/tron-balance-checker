package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/sxwebdev/gotron/pkg/address"
	"github.com/sxwebdev/xconfig"
	"github.com/sxwebdev/xconfig/plugins/loader"
	"gopkg.in/yaml.v3"
)

type NodeConfig struct {
	GrpcAddr string `yaml:"grpc_addr"`
	Headers  string `yaml:"headers"`
	UseTLS   bool   `yaml:"use_tls" default:"true"`
}

type USDTConfig struct {
	Contract string `yaml:"contract" default:"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"`
	Decimals int32  `yaml:"decimals" default:"6"`
}

type CheckerConfig struct {
	RateLimit      int           `yaml:"rate_limit" default:"3" env:"RATE_LIMIT"`
	BatchSize      int           `yaml:"batch_size" default:"50" env:"BATCH_SIZE"`
	RetryMax       int           `yaml:"retry_max" default:"3" env:"RETRY_MAX"`
	RetryDelay     time.Duration `yaml:"retry_delay" default:"1s" env:"RETRY_DELAY"`
	RequestTimeout time.Duration `yaml:"request_timeout" default:"15s" env:"REQUEST_TIMEOUT"`
	LoopPeriod     time.Duration `yaml:"loop_period" default:"100ms" env:"LOOP_PERIOD"`
}

type DatabaseConfig struct {
	Path string `yaml:"path" default:"data/sqlite/db.sqlite" env:"DB_PATH"`
}

type Config struct {
	Nodes    []NodeConfig   `yaml:"nodes"`
	USDT     USDTConfig     `yaml:"usdt"`
	Checker  CheckerConfig  `yaml:"checker"`
	Database DatabaseConfig `yaml:"database"`
}

func (c *Config) Validate() error {
	if len(c.Nodes) == 0 {
		return errors.New("config: nodes must contain at least one entry")
	}
	for i, n := range c.Nodes {
		if n.GrpcAddr == "" {
			return fmt.Errorf("config: nodes[%d].grpc_addr is required", i)
		}
	}
	if c.Checker.RateLimit < 1 {
		return errors.New("config: checker.rate_limit must be >= 1")
	}
	if c.Checker.BatchSize < 1 {
		return errors.New("config: checker.batch_size must be >= 1")
	}
	if c.Checker.RetryMax < 1 {
		return errors.New("config: checker.retry_max must be >= 1")
	}
	if c.Database.Path == "" {
		return errors.New("config: database.path is required")
	}
	if err := address.Validate(c.USDT.Contract); err != nil {
		return fmt.Errorf("config: usdt.contract is not a valid Tron address: %w", err)
	}
	if c.USDT.Decimals < 0 {
		return errors.New("config: usdt.decimals must be >= 0")
	}
	return nil
}

func Load(path string) (*Config, error) {
	l, err := loader.NewLoader(map[string]loader.Unmarshal{
		"yaml": yaml.Unmarshal,
		"yml":  yaml.Unmarshal,
	})
	if err != nil {
		return nil, fmt.Errorf("init config loader: %w", err)
	}

	if path != "" {
		if err := l.AddFile(path, false); err != nil {
			return nil, fmt.Errorf("add config file %q: %w", path, err)
		}
	}

	cfg := &Config{}
	if _, err := xconfig.Load(cfg,
		xconfig.WithLoader(l),
		xconfig.WithEnvPrefix("TBC"),
	); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return cfg, nil
}
