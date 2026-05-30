package registry

import (
	"context"
)

type Registry interface {
	Allocate(ctx context.Context, timestamp, blockSize int64) (start int64, err error)
}
