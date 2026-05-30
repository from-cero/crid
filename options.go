package crid

import (
	"fmt"
	"strings"
	"time"
)

// Option configures a Node, Parser at creation time.
type Option func(*config)

// WithFormat sets the bit layout of generated IDs using the given format options.
// By default, the timestamp/sequence split from defaultFormat is used.
func WithFormat(opts ...FormatOption) Option {
	return func(c *config) { c.format = applyFormatOptions(opts) }
}

// WithEpoch sets the reference time from which timestamps are measured.
// The epoch must not be in the future. The default is 2026-01-01 00:00:00 UTC.
func WithEpoch(e time.Time) Option {
	return func(c *config) { c.epoch = e }
}

// WithBlockSize sets the number of sequence numbers a Node reserves from the
// registry per allocation. It must be positive. The default is 10,000.
func WithBlockSize(b int64) Option {
	return func(c *config) { c.blockSize = b }
}

type config struct {
	format    format    // Default is crid.defaultFormat
	epoch     time.Time // Default is 2026-01-01 00:00:00 UTC
	blockSize int64     // Default is 10,000
}

var defaultConfig = config{
	format:    defaultFormat,
	epoch:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	blockSize: 10_000,
}

func applyOptions(opts []Option) config {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

func (c *config) validate() error {
	var errs []string
	if err := c.format.validate(); err != nil {
		errs = append(errs, err.Error())
	}
	if c.epoch.After(time.Now()) {
		errs = append(errs, ErrEpochInFuture.Error())
	}
	if c.blockSize < 1 {
		errs = append(errs, ErrInvalidBlockSize.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", ErrInvalidConfig, strings.Join(errs, ", "))
	}
	return nil
}
