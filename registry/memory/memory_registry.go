package memory

import (
	"context"
	"sync"

	"github.com/from-cero/crid/registry"
)

var _ registry.Registry = (*Registry)(nil)

// Registry is an in-memory registry suitable for tests and single-process deployments.
// Allocations are not persisted across restarts.
type Registry struct {
	mu   sync.Mutex
	next map[int64]int64
}

// New returns a ready-to-use in-memory Registry.
func New() *Registry {
	return &Registry{next: make(map[int64]int64)}
}

// Allocate reserves blockSize sequence numbers for timestamp
// and returns the starting value of the reserved block.
func (r *Registry) Allocate(_ context.Context, timestamp, blockSize int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	start := r.next[timestamp]
	r.next[timestamp] = start + blockSize
	return start, nil
}
