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
	mu    sync.Mutex
	ts    int64
	seq   int64
	limit int64

	cfg       config
	comF      compiledFormat
	reg       registry.Registry
	epochS    int64
	blockSize int64
}

// New creates a Node backed by reg, applying any options on top of the defaults.
func New(reg registry.Registry, opts ...Option) (*Node, error) {
	cfg := applyOptions(opts)
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	comF := cfg.format.compileFormat()
	epochS := cfg.epoch.Unix()
	blockSize := cfg.blockSize

	if reg == nil {
		return nil, ErrNilRegistry
	}

	return &Node{
		seq:   0,
		limit: -1,

		cfg:       cfg,
		comF:      comF,
		reg:       reg,
		epochS:    epochS,
		blockSize: blockSize,
	}, nil
}

// Generate returns the next unique ID.
// Reserving a new block of sequence numbers from the registry when the current allocation is exhausted.
func (n *Node) Generate(ctx context.Context) (ID, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// allocated range is exhausted
	if n.seq > n.limit {
		if err := n.refill(ctx); err != nil {
			return 0, err
		}
	}

	if n.seq < 0 || n.seq > n.comF.maxSeq {
		return 0, ErrInvalidSequence
	}
	// check whether n.limit still be behind n.seq
	if n.seq > n.limit {
		return 0, ErrAllocationRefillFailed
	}

	var idI64 int64
	idI64 |= n.ts << n.comF.shiftTimestamp
	idI64 |= n.seq
	n.seq++
	return ID(idI64), nil
}

func (n *Node) refill(ctx context.Context) error {
	now := n.nowS()
	if now < 0 {
		return ErrClockBeforeEpoch
	}
	if now > n.comF.maxTimestamp {
		return ErrTimestampOverflow
	}

	start, err := n.reg.Allocate(ctx, now, n.blockSize)
	if err != nil {
		return fmt.Errorf("could not refill allocation: %w", err)
	}
	if start > n.comF.maxSeq {
		return ErrSequenceOverflow
	}

	n.ts = now
	n.seq = start
	n.limit = min(start+n.blockSize-1, n.comF.maxSeq)
	return nil
}

func (n *Node) nowS() int64 {
	return time.Now().Unix() - n.epochS
}
