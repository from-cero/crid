package crid

import (
	"errors"
	"testing"
	"time"
)

func TestApplyOptions_Defaults(t *testing.T) {
	cfg := applyOptions(nil)
	if cfg.format != defaultConfig.format {
		t.Errorf("format = %+v, want %+v", cfg.format, defaultConfig.format)
	}
	if !cfg.epoch.Equal(defaultConfig.epoch) {
		t.Errorf("epoch = %v, want %v", cfg.epoch, defaultConfig.epoch)
	}
	if cfg.blockSize != defaultConfig.blockSize {
		t.Errorf("blockSize = %d, want %d", cfg.blockSize, defaultConfig.blockSize)
	}
	if cfg.threshold != defaultConfig.threshold {
		t.Errorf("threshold = %d, want %d", cfg.threshold, defaultConfig.threshold)
	}
}

func TestApplyOptions_Overrides(t *testing.T) {
	epoch := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	cfg := applyOptions([]Option{
		WithFormat(WithTimestampBits(41), WithSequenceBits(22)),
		WithEpoch(epoch),
		WithBlockSize(500),
		WithThreshold(100),
	})
	if cfg.format.timestampBits != 41 || cfg.format.sequenceBits != 22 {
		t.Errorf("format = %+v, want 41/22", cfg.format)
	}
	if !cfg.epoch.Equal(epoch) {
		t.Errorf("epoch = %v, want %v", cfg.epoch, epoch)
	}
	if cfg.blockSize != 500 {
		t.Errorf("blockSize = %d, want 500", cfg.blockSize)
	}
	if cfg.threshold != 100 {
		t.Errorf("threshold = %d, want 100", cfg.threshold)
	}
}

func TestConfig_Validate(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)

	tests := []struct {
		name     string
		mutate   func(*config)
		wantErrs []error
	}{
		{
			name:   "valid default",
			mutate: func(*config) {},
		},
		{
			name:     "invalid bit format",
			mutate:   func(c *config) { c.format = format{timestampBits: 1, sequenceBits: 1} },
			wantErrs: []error{ErrInvalidBitFormat},
		},
		{
			name:     "epoch in future",
			mutate:   func(c *config) { c.epoch = future },
			wantErrs: []error{ErrEpochInFuture},
		},
		{
			name:     "zero block size",
			mutate:   func(c *config) { c.blockSize = 0 },
			wantErrs: []error{ErrInvalidBlockSize},
		},
		{
			name:     "negative block size",
			mutate:   func(c *config) { c.blockSize = -5 },
			wantErrs: []error{ErrInvalidBlockSize},
		},
		{
			name:     "threshold exceeds block size",
			mutate:   func(c *config) { c.threshold = c.blockSize + 1 },
			wantErrs: []error{ErrInvalidThreshold},
		},
		{
			name: "multiple errors collected",
			mutate: func(c *config) {
				c.format = format{timestampBits: 1, sequenceBits: 1}
				c.epoch = future
				c.blockSize = 0
			},
			wantErrs: []error{ErrInvalidBitFormat, ErrEpochInFuture, ErrInvalidBlockSize},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultConfig
			tt.mutate(&cfg)
			err := cfg.validate()

			if len(tt.wantErrs) == 0 {
				if err != nil {
					t.Fatalf("validate() = %v, want nil", err)
				}
				return
			}

			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("validate() should wrap ErrInvalidConfig, got %v", err)
			}
			for _, want := range tt.wantErrs {
				if !errors.Is(err, want) {
					t.Errorf("validate() = %v, want it to wrap %v", err, want)
				}
			}
		})
	}
}

func TestWithThreshold_BelowOneDisablesPrealloc(t *testing.T) {
	// A threshold below 1 is allowed by validation; it disables pre-allocation.
	cfg := applyOptions([]Option{WithThreshold(0)})
	if err := cfg.validate(); err != nil {
		t.Errorf("validate() = %v, want nil for threshold 0", err)
	}
}
