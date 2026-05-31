package crid

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/from-cero/crid/registry"
)

// Node generates unique IDs by combining the timestamp at which a block of sequence numbers
// was reserved from the Registry with the sequence numbers in that block. The embedded
// timestamp therefore reflects block-allocation time, not the moment an individual ID is
// generated, so IDs are not strictly ordered by generation time.
// A Node is safe for concurrent use by multiple goroutines.
type Node struct {
	mu            sync.Mutex
	ts            int64
	seq           int64
	limit         int64
	preAlloc      *allocation
	preAllocating bool

	cfg       config
	comF      compiledFormat
	reg       registry.Registry
	epochS    int64
	blockSize int64
	threshold int64
}

// New creates a Node backed by reg, applying any options on top of the defaults.
func New(reg registry.Registry, opts ...Option) (*Node, error) {
	cfg := applyOptions(opts)
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	if reg == nil {
		return nil, ErrNilRegistry
	}

	return &Node{
		ts:            0,
		seq:           0,
		limit:         -1,
		preAlloc:      nil,
		preAllocating: false,

		cfg:       cfg,
		comF:      cfg.format.compileFormat(),
		reg:       reg,
		epochS:    cfg.epoch.Unix(),
		blockSize: cfg.blockSize,
		threshold: cfg.threshold,
	}, nil
}

// Generate returns the next unique ID, reserving a new block of sequence numbers
// from the registry when the current allocation is exhausted.
func (n *Node) Generate(ctx context.Context) (ID, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// pre-allocate the next range once remaining drops below threshold
	remaining := n.limit - n.seq + 1
	if n.limit >= 0 && remaining < n.threshold {
		if n.preAlloc == nil && !n.preAllocating {
			n.preAllocating = true
			go n.prefill(context.Background())
		}
	}

	// allocated range is exhausted
	if n.seq > n.limit {
		if err := n.refill(ctx); err != nil {
			return ID(-1), err
		}
	}

	if n.seq < 0 || n.seq > n.comF.maxSeq {
		return ID(-1), ErrInvalidSequence
	}

	var idI64 int64
	idI64 |= n.ts << n.comF.shiftTimestamp
	idI64 |= n.seq
	n.seq++
	return ID(idI64), nil
}

func (n *Node) refill(ctx context.Context) error {
	if n.preAlloc != nil {
		// this algorithm focuses on the ID uniqueness and correctness
		// so the change stale ts when updating n.ts with preAlloc.ts is accepted
		n.ts = n.preAlloc.timestamp
		n.seq = n.preAlloc.start
		n.limit = min(n.preAlloc.end-1, n.comF.maxSeq)
		n.preAlloc = nil
		return nil
	}

	alloc, err := n.allocate(ctx)
	if err != nil {
		return fmt.Errorf("allocate block failed: %w", err)
	}

	n.ts = alloc.timestamp
	n.seq = alloc.start
	n.limit = min(alloc.end-1, n.comF.maxSeq)
	return nil
}

func (n *Node) prefill(ctx context.Context) {
	alloc, err := n.allocate(ctx)
	if err != nil {
		n.mu.Lock()
		n.preAllocating = false
		n.mu.Unlock()
		return // ignore error
	}

	// need lock before read/write to Node
	// because prefill run on a separate goroutine
	// and may update preAlloc while Generate is running
	n.mu.Lock()
	n.preAlloc = alloc
	n.preAllocating = false
	n.mu.Unlock()
}

func (n *Node) allocate(ctx context.Context) (*allocation, error) {
	now := n.nowS()
	if now < 0 {
		return nil, ErrClockBeforeEpoch
	}
	if now > n.comF.maxTimestamp {
		return nil, ErrTimestampOverflow
	}

	start, err := n.reg.Allocate(ctx, now, n.blockSize)
	if err != nil {
		return nil, fmt.Errorf("could not refill allocation: %w", err)
	}
	if start > n.comF.maxSeq {
		return nil, ErrSequenceOverflow
	}

	return &allocation{
		timestamp: now,
		start:     start,
		end:       start + n.blockSize,
	}, nil
}

func (n *Node) nowS() int64 {
	return time.Now().Unix() - n.epochS
}
