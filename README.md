# crid

`crid` is a Snowflake-variant distributed ID generator for Go. It produces unique 63-bit integer IDs by combining a timestamp with a sequence number drawn from a block reserved from a shared registry. Nodes need no per-node identity -- uniqueness comes from the registry handing out non-overlapping blocks per timestamp.

**Highlights**: 63-bit IDs that fit in `int64`; high-throughput block reservation with async pre-allocation; pluggable registry (in-memory, Postgres, or custom); configurable bit layout, epoch, block size, and threshold.

## Install

```sh
go get -u github.com/from-cero/crid
```

Requires Go 1.26 or newer.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/from-cero/crid"
	"github.com/from-cero/crid/registry/memory"
)

func main() {
	ctx := context.Background()

	reg := memory.New()
	node, err := crid.New(reg)
	if err != nil {
		log.Fatal(err)
	}

	parser, err := crid.NewParser()
	if err != nil {
		log.Fatal(err)
	}

	id, err := node.Generate(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s -> %s\n", id, parser.Parse(id))
}
```

## How it works

1. `Node` calls `Registry.Allocate(ctx, timestamp, blockSize)` to reserve a block of sequence numbers for the current second.
2. `Generate` serves IDs from that block in memory: `id = (timestamp << sequenceBits) | sequence`.
3. When the remaining count drops below `threshold`, the next block is reserved in the background. If the current block is exhausted before pre-allocation completes, `Generate` falls back to a synchronous call.

Two IDs can only collide if they share both timestamp and sequence number, which the registry prevents.

## ID layout

```
 63                    sequenceBits             0
+--+-------------------+------------------------+
|0 |     timestamp     |        sequence        |
+--+-------------------+------------------------+
```

Default: 31 timestamp bits (~68 years from epoch) and 32 sequence bits (~4.3 billion IDs/second). `timestampBits + sequenceBits` must equal 63.

### Working with IDs

```go
id.Int64()  // raw int64
id.String() // decimal string
```

`ID` marshals to/from JSON as a quoted decimal string to avoid precision loss in JavaScript:

```json
{ "id": "123456789012345" }
```

## Parsing

```go
parser, err := crid.NewParser() // must use same epoch and format as the generating Node
parsed := parser.Parse(id)
parsed.Timestamp // time.Time
parsed.Sequence  // int64
```

> [!IMPORTANT]
> A `Parser` must be created with the **same epoch and format** as the `Node` that generated
> the IDs. A mismatch produces silently wrong results, not an error.

## Configuration

Pass options to `New` and `NewParser`:

| Option | Default | Description |
|--------|---------|-------------|
| `WithFormat(WithTimestampBits, WithSequenceBits)` | 31 / 32 | Bit split; must sum to 63. |
| `WithEpoch(time.Time)` | 2026-01-01 00:00:00 UTC | Reference time; must not be in the future. |
| `WithBlockSize(int64)` | 10,000 | Sequence numbers reserved per registry call. Must be in `[1, 2^sequenceBits]`. |
| `WithThreshold(int64)` | 5,000 | Remaining count that triggers background pre-allocation. Must not exceed block size; below 1 disables pre-allocation. |

```go
node, err := crid.New(reg,
	crid.WithEpoch(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
	crid.WithFormat(crid.WithTimestampBits(41), crid.WithSequenceBits(22)),
	crid.WithBlockSize(1_000),
	crid.WithThreshold(200),
)
```

**Tuning:** Keep `threshold >= peak_rate * registry_latency` so background pre-allocation stays ahead of demand. Larger blocks mean fewer round-trips but larger wasted gaps if a node restarts mid-block.

## Registry

```go
type Registry interface {
	// Allocate reserves blockSize sequence numbers for timestamp and returns
	// the starting value of the block. Successive calls for the same timestamp
	// must return non-overlapping blocks.
	Allocate(ctx context.Context, timestamp, blockSize int64) (start int64, err error)
}
```

### Memory

```go
import "github.com/from-cero/crid/registry/memory"

reg := memory.New()
```

Suitable for tests and single-process deployments. Does not persist allocations across restarts.

### Postgres

For durable, multi-process deployments. A separate module so the core stays dependency-free.

```sh
go get -u github.com/from-cero/crid/registry/postgres
```

Requires Go 1.26+ and `github.com/jackc/pgx/v5` v5.10.0+.

```go
import (
	"github.com/from-cero/crid/registry/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

pool, err := pgxpool.New(ctx, dsn)
if err != nil {
	log.Fatal(err)
}
defer pool.Close()

reg, err := postgres.New(pool, postgres.WithTable("crid_example_allocations"))
if err != nil {
	log.Fatal(err)
}
// Create the table once at startup, or manage it with migrations instead.
if err := reg.EnsureSchema(ctx); err != nil {
	log.Fatal(err)
}

node, err := crid.New(reg)
```

`New` accepts `*pgxpool.Pool`, `*pgx.Conn`, or `pgx.Tx`. The table name defaults to `crid_allocations`; override with `postgres.WithTable("schema.table")`. Each `Allocate` is a single atomic UPSERT, so concurrent callers across all processes receive non-overlapping blocks:

```sql
CREATE TABLE IF NOT EXISTS crid_allocations (
    ts       BIGINT PRIMARY KEY,
    next_seq BIGINT NOT NULL
);
```

## Guarantees and caveats

- **Uniqueness** is guaranteed for all nodes sharing one registry, as long as the registry honors non-overlapping blocks per timestamp.
- **IDs are not time-ordered.** The timestamp reflects when the block was reserved, not when the ID was generated. Async pre-allocation means an ID can carry a slightly earlier timestamp. Do not rely on monotonicity or k-sortability.
- **Error sentinel.** `Generate` returns `ID(-1)` on failure. Always check the returned error; do not treat `0` as "no ID" (zero is a valid ID).
- **Clock.** A backward clock jump below the epoch returns `ErrClockBeforeEpoch`. Uniqueness is never affected by clock movement.

## Errors

`New`/`NewParser` validation errors wrap `ErrInvalidConfig`: `ErrInvalidBitFormat`, `ErrEpochInFuture`, `ErrInvalidBlockSize`, `ErrInvalidThreshold`. `New` also returns `ErrNilRegistry`.

`Generate` may return `ErrClockBeforeEpoch`, `ErrTimestampOverflow`, `ErrSequenceOverflow`, or `ErrInvalidSequence`, plus any registry error. Use `errors.Is` to test them.

## Concurrency

`Node` is safe for concurrent use by multiple goroutines. Background pre-allocation runs in its own goroutine and synchronizes on the node's internal lock.
