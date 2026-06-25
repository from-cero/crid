package crid

import (
	"errors"
	"fmt"
	"time"
)

// Option configures a Node or Parser at creation time.
type Option func(*config)

// WithFormat sets the bit layout of generated IDs using the given format options.
func WithFormat(opts ...FormatOption) Option {
	return func(c *config) { c.format = applyFormatOptions(opts) }
}

// WithEpoch sets the reference time from which timestamps are measured.
// The epoch must not be in the future. The default is 2026-01-01 00:00:00 UTC.
func WithEpoch(e time.Time) Option { return func(c *config) { c.epoch = e } }

// WithBlockSize sets the number of sequence numbers a Node reserves from the
// registry per allocation. It must be positive. The default is 10,000.
func WithBlockSize(b int64) Option { return func(c *config) { c.blockSize = b } }

// WithThreshold sets how many sequence numbers must remain in the current block
// before the Node asynchronously pre-allocates the next one. It must not exceed the
// block size. A value below 1 disables pre-allocation. The default is 5,000.
func WithThreshold(t int64) Option { return func(c *config) { c.threshold = t } }

type config struct {
	format    format    // Default is crid.defaultFormat
	epoch     time.Time // Default is 2026-01-01 00:00:00 UTC
	blockSize int64     // Default is 10,000
	threshold int64     // Default is 5,000
}

var defaultConfig = config{
	format:    defaultFormat,
	epoch:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	blockSize: 10_000,
	threshold: 5_000,
}

func applyOptions(opts []Option) *config {
	cfg := defaultConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return &cfg
}

func (c *config) validate() error {
	var errs []error
	if err := c.format.validate(); err != nil {
		errs = append(errs, err)
	}
	if c.epoch.After(time.Now()) {
		errs = append(errs, ErrEpochInFuture)
	}
	if c.blockSize < 1 || c.blockSize > (int64(1)<<c.format.sequenceBits) {
		errs = append(errs, ErrInvalidBlockSize)
	}
	if c.threshold > c.blockSize {
		errs = append(errs, ErrInvalidThreshold)
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidConfig, errors.Join(errs...))
	}
	return nil
}
