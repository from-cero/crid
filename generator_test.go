package crid

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/from-cero/crid/registry"
	"github.com/from-cero/crid/registry/memory"
)

// fakeRegistry lets tests control the start value and error returned by Allocate.
type fakeRegistry struct {
	mu        sync.Mutex
	allocFn   func(timestamp, blockSize int64) (int64, error)
	callCount int
}

func (f *fakeRegistry) Allocate(_ context.Context, timestamp, blockSize int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	return f.allocFn(timestamp, blockSize)
}

var _ registry.Registry = (*fakeRegistry)(nil)

func TestNew_NilRegistry(t *testing.T) {
	n, err := New(nil)
	if !errors.Is(err, ErrNilRegistry) {
		t.Errorf("New(nil) error = %v, want ErrNilRegistry", err)
	}
	if n != nil {
		t.Error("New(nil) returned non-nil node")
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	n, err := New(memory.New(), WithBlockSize(0))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("New() error = %v, want ErrInvalidConfig", err)
	}
	if n != nil {
		t.Error("New() returned non-nil node on invalid config")
	}
}

func TestNew_Valid(t *testing.T) {
	n, err := New(memory.New())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if n == nil {
		t.Fatal("New() returned nil node")
	}
}

func TestGenerate_Uniqueness(t *testing.T) {
	tests := []struct {
		name      string
		blockSize int64
		threshold int64
		count     int
	}{
		{"no prefill, refills across blocks", 10, 0, 35},
		{"no prefill, large count", 100, 0, 1000},
		{"with prefill", 10, 5, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := New(memory.New(), WithBlockSize(tt.blockSize), WithThreshold(tt.threshold))
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			seen := make(map[ID]struct{}, tt.count)
			for i := range tt.count {
				id, err := n.Generate(context.Background())
				if err != nil {
					t.Fatalf("Generate() error = %v", err)
				}
				if _, dup := seen[id]; dup {
					t.Fatalf("duplicate ID %d at iteration %d", id, i)
				}
				seen[id] = struct{}{}
			}
		})
	}
}

func TestGenerate_RegistryError(t *testing.T) {
	wantErr := errors.New("boom")
	reg := &fakeRegistry{allocFn: func(_, _ int64) (int64, error) {
		return 0, wantErr
	}}
	n, err := New(reg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = n.Generate(context.Background())
	if !errors.Is(err, wantErr) {
		t.Errorf("Generate() error = %v, want it to wrap %v", err, wantErr)
	}
}

func TestGenerate_SequenceOverflow(t *testing.T) {
	// sequenceBits = 1 means maxSeq = 1; a start above that overflows.
	reg := &fakeRegistry{allocFn: func(_, _ int64) (int64, error) {
		return 1_000_000, nil
	}}
	// blockSize must not exceed the format's max sequence number (1 here), and
	// threshold must not exceed blockSize, so pin both to keep the config valid.
	n, err := New(reg, WithFormat(WithTimestampBits(62), WithSequenceBits(1)), WithBlockSize(1), WithThreshold(0))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = n.Generate(context.Background())
	if !errors.Is(err, ErrSequenceOverflow) {
		t.Errorf("Generate() error = %v, want ErrSequenceOverflow", err)
	}
}

func TestGenerate_TimestampOverflow(t *testing.T) {
	// timestampBits = 1 means maxTimestamp = 1; seconds since the 2026 epoch
	// vastly exceed that, so allocation fails before touching the registry.
	reg := &fakeRegistry{allocFn: func(_, _ int64) (int64, error) {
		t.Error("registry should not be called on timestamp overflow")
		return 0, nil
	}}
	n, err := New(reg, WithFormat(WithTimestampBits(1), WithSequenceBits(62)))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = n.Generate(context.Background())
	if !errors.Is(err, ErrTimestampOverflow) {
		t.Errorf("Generate() error = %v, want ErrTimestampOverflow", err)
	}
}

func TestGenerate_PrefillErrorIgnored(t *testing.T) {
	// First Allocate (the synchronous refill) succeeds; the async prefill fails.
	// The failure must be swallowed and generation must keep working via refill.
	reg := &fakeRegistry{}
	reg.allocFn = func(_, _ int64) (int64, error) {
		if reg.callCount == 1 {
			return 0, nil
		}
		return 0, errors.New("prefill boom")
	}
	n, err := New(reg, WithBlockSize(10), WithThreshold(5))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Drain the first block; the prefill attempt fails but Generate must still succeed.
	for range 10 {
		if _, err := n.Generate(context.Background()); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
	}

	// Give the background prefill goroutine a chance to run and clear preAllocating.
	for range 100 {
		runtime.Gosched()
	}

	// Next block can no longer come from a successful prefill; a refill error surfaces.
	if _, err := n.Generate(context.Background()); err == nil {
		t.Error("Generate() error = nil, want refill error after block exhausted")
	}
}

func TestGenerate_Concurrent(t *testing.T) {
	n, err := New(memory.New(), WithBlockSize(50), WithThreshold(25))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	const goroutines = 20
	const perGoroutine = 200

	var (
		mu   sync.Mutex
		seen = make(map[ID]struct{}, goroutines*perGoroutine)
		wg   sync.WaitGroup
	)
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range perGoroutine {
				id, err := n.Generate(context.Background())
				if err != nil {
					t.Errorf("Generate() error = %v", err)
					return
				}
				mu.Lock()
				if _, dup := seen[id]; dup {
					t.Errorf("duplicate ID %d", id)
				}
				seen[id] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != goroutines*perGoroutine {
		t.Errorf("generated %d unique IDs, want %d", len(seen), goroutines*perGoroutine)
	}
}

func TestGenerate_ParseRoundTrip(t *testing.T) {
	epoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	n, err := New(memory.New(), WithEpoch(epoch), WithThreshold(0))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	p, err := NewParser(WithEpoch(epoch))
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}

	before := time.Now()
	id, err := n.Generate(context.Background())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	after := time.Now()

	parsed := p.Parse(id)
	if parsed.Sequence != 0 {
		t.Errorf("first Sequence = %d, want 0", parsed.Sequence)
	}
	if parsed.Timestamp.Before(before.Add(-time.Second)) || parsed.Timestamp.After(after.Add(time.Second)) {
		t.Errorf("Timestamp = %v out of range [%v, %v]", parsed.Timestamp, before, after)
	}
}
