package registry

import (
	"context"
)

// Registry hands out non-overlapping blocks of sequence numbers for a given timestamp since the epoch,
// allowing multiple nodes to generate unique IDs without coordination.
type Registry interface {
	// Allocate reserves blockSize sequence numbers for timestamp and returns the
	// starting value of the reserved block. Successive calls for the same timestamp
	// must return non-overlapping blocks.
	Allocate(ctx context.Context, timestamp, blockSize int64) (start int64, err error)
}
