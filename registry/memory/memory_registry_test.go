package memory

import (
	"context"
	"sync"
	"testing"
)

func TestAllocate_SequentialNonOverlapping(t *testing.T) {
	r := New()
	ctx := context.Background()
	const ts = 42

	start1, err := r.Allocate(ctx, ts, 100)
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	if start1 != 0 {
		t.Errorf("first start = %d, want 0", start1)
	}

	start2, err := r.Allocate(ctx, ts, 100)
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	if start2 != 100 {
		t.Errorf("second start = %d, want 100", start2)
	}

	start3, err := r.Allocate(ctx, ts, 50)
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	if start3 != 200 {
		t.Errorf("third start = %d, want 200", start3)
	}
}

func TestAllocate_IndependentTimestamps(t *testing.T) {
	r := New()
	ctx := context.Background()

	if _, err := r.Allocate(ctx, 1, 100); err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	// A different timestamp keeps its own counter starting at 0.
	start, err := r.Allocate(ctx, 2, 100)
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	if start != 0 {
		t.Errorf("start for new timestamp = %d, want 0", start)
	}
}

func TestAllocate_Concurrent(t *testing.T) {
	r := New()
	ctx := context.Background()
	const ts = 7
	const goroutines = 50
	const blockSize = 10

	var (
		mu     sync.Mutex
		starts = make(map[int64]struct{}, goroutines)
		wg     sync.WaitGroup
	)
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			start, err := r.Allocate(ctx, ts, blockSize)
			if err != nil {
				t.Errorf("Allocate() error = %v", err)
				return
			}
			mu.Lock()
			if _, dup := starts[start]; dup {
				t.Errorf("overlapping start %d handed out twice", start)
			}
			starts[start] = struct{}{}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(starts) != goroutines {
		t.Errorf("got %d distinct starts, want %d", len(starts), goroutines)
	}
}
