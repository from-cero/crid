# Custom Range-based ID Generator for Go

`crid` is a range-based, Snowflake-variant distributed ID generator for Go. It produces
unique 63-bit integer IDs by combining a timestamp with a sequence number drawn from a
block reserved out of a shared registry.

Uniqueness comes entirely from the registry, which hands out non-overlapping blocks of
sequence numbers per timestamp. Nodes need no identity of their own and require no
coordination with one another beyond the registry they share.

## Features

- 63-bit IDs that fit in an `int64` and are always non-negative.
- High throughput: a node reserves a block of sequence numbers at a time and serves them
  from memory, hitting the registry only once per block.
- Asynchronous pre-allocation: the next block is reserved in the background before the
  current one runs out, so steady-state generation does not block on the registry.
- Pluggable registry: an in-memory implementation ships with the library, a durable
  Postgres-backed one is available under `registry/postgres`, and any backend that
  satisfies the `registry.Registry` interface works.
- Configurable bit layout, epoch, block size, and pre-allocation threshold.

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

	// All nodes that must not collide share one registry.
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

1. A `Node` asks the `Registry` to reserve `blockSize` sequence numbers for the current
   second (`Allocate(ctx, secondsSinceEpoch, blockSize)`), receiving the start of the block.
2. `Generate` hands out IDs from that block in memory: `id = (timestamp << sequenceBits) | sequence`.
3. When the remaining count in the block drops below `threshold`, the node reserves the
   next block in a background goroutine. When the current block is exhausted it swaps in
   the pre-allocated block, or reserves one synchronously if the background reservation has
   not finished.

Because the registry guarantees non-overlapping blocks per timestamp, two IDs can only
collide if they share both timestamp and sequence number, which never happens. This holds
across every node sharing the registry, with no per-node ID.

## ID layout

An ID is a 63-bit value packed into an `int64` (the sign bit is always zero, so valid IDs
are non-negative):

```
 63                    sequenceBits             0
+--+-------------------+------------------------+
|0 |     timestamp     |        sequence        |
+--+-------------------+------------------------+
```

The default layout is 31 timestamp bits and 32 sequence bits:

- 31 timestamp bits in seconds give roughly 68 years of range from the epoch.
- 32 sequence bits allow about 4.29 billion IDs per second across all nodes sharing the
  registry.

`timestampBits + sequenceBits` must always equal 63.

### Working with IDs

```go
id.Int64() // the raw int64
id.String() // decimal string
```

`ID` marshals to and from JSON as a quoted decimal string, avoiding precision loss in
environments (such as JavaScript) that cannot represent 63-bit integers exactly:

```json
{
  "id": "123456789012345"
}
```

## Parsing

A `Parser` decodes an ID back into its timestamp and sequence without a running node:

```go
parsed := parser.Parse(id)
parsed.Timestamp // time.Time
parsed.Sequence  // int64
```

> [!IMPORTANT]
> A `Parser` must be created with the **same epoch and format** as the `Node` that
> generated the IDs. A mismatch produces silently incorrect results, not an error.

## Configuration

Pass options to `New` and `NewParser`:

| Option                                            | Default                 | Description                                                                                                                       |
|---------------------------------------------------|-------------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| `WithFormat(WithTimestampBits, WithSequenceBits)` | 31 / 32                 | Bit split; must sum to 63.                                                                                                        |
| `WithEpoch(time.Time)`                            | 2026-01-01 00:00:00 UTC | Reference time. Must not be in the future.                                                                                        |
| `WithBlockSize(int64)`                            | 10,000                  | Sequence numbers reserved per registry call. Must be in `[1, 2^sequenceBits]`.                                                    |
| `WithThreshold(int64)`                            | 5,000                   | Remaining count that triggers background pre-allocation. Must not exceed the block size; a value below 1 disables pre-allocation. |

```go
node, err := crid.New(reg,
    crid.WithEpoch(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
    crid.WithFormat(crid.WithTimestampBits(41), crid.WithSequenceBits(22)),
    crid.WithBlockSize(1_000),
    crid.WithThreshold(200),
)
```

### Tuning block size and threshold

A larger `blockSize` means fewer registry round-trips but coarser allocation (and, with a
remote registry, larger permanently-wasted gaps if a node restarts mid-block). The
`threshold` should leave enough headroom to reserve the next block before the current one
is exhausted. As a rule of thumb, keep:

```
threshold >= peak_generation_rate * registry_allocation_latency
```

so background pre-allocation stays ahead of demand and `Generate` does not fall back to a
synchronous registry call.

## Registry

```go
type Registry interface {
    // Allocate reserves blockSize sequence numbers for timestamp and returns the
    // starting value of the reserved block. Successive calls for the same timestamp
    // must return non-overlapping blocks.
    Allocate(ctx context.Context, timestamp, blockSize int64) (start int64, err error)
}
```

The bundled `registry/memory` implementation is suitable for tests, examples, and
single-process deployments. It does not persist allocations across restarts.

### Memory

```go
import "github.com/from-cero/crid/registry/memory"

reg := memory.New()
```

### Postgres

For durable, multi-process deployments, `registry/postgres` backs the registry with
PostgreSQL. Allocations are persisted, so a block is never handed out twice even across
restarts, and uniqueness holds for every node sharing the same database. It is a separate
module (so the core stays dependency-free) and uses the [pgx](https://github.com/jackc/pgx)
driver:

```sh
go get -u github.com/from-cero/crid/registry/postgres
```

```go
import (
    "github.com/from-cero/crid"
    "github.com/from-cero/crid/registry/postgres"
    "github.com/jackc/pgx/v5/pgxpool"
)

pool, err := pgxpool.New(ctx, dsn)
if err != nil {
    log.Fatal(err)
}
defer pool.Close()

reg, err := postgres.New(pool)
if err != nil {
    log.Fatal(err)
}
// Create the table once at startup, or manage it with migrations instead.
if err := reg.EnsureSchema(ctx); err != nil {
    log.Fatal(err)
}

node, err := crid.New(reg)
```

`New` accepts any value satisfying its `DB` interface (`*pgxpool.Pool`, `*pgx.Conn`,
or `pgx.Tx`). The table name defaults to `crid_allocations` and can be overridden with
`postgres.WithTable("schema.table")`. Each `Allocate` is a single atomic UPSERT, so
concurrent callers across all processes receive non-overlapping blocks. The schema is:

```sql
CREATE TABLE IF NOT EXISTS crid_allocations (
    ts       BIGINT PRIMARY KEY,
    next_seq BIGINT NOT NULL
);
```

A durable registry on any other backend can be added the same way, by implementing the
`registry.Registry` interface; the node algorithm is unchanged.

## Guarantees and caveats

- **Uniqueness** is guaranteed for all nodes sharing one registry, as long as the registry
  honors its contract (non-overlapping blocks per timestamp).
- **IDs are not time-ordered.** The embedded timestamp is the time the block was reserved,
  not the time an individual ID was generated. Because the next block is reserved ahead of
  time, an ID can carry a timestamp from slightly earlier than its actual generation moment.
  Do not rely on these IDs being monotonic or k-sortable.
- **Error sentinel.** On failure `Generate` returns `ID(-1)`. Valid IDs are always
  non-negative, so a negative result unambiguously signals an error. Always check the
  returned error; do not treat `0` as "no ID" (zero is a valid ID).
- **Clock.** Timestamps come from the system wall clock. A backward clock jump below the
  epoch causes `ErrClockBeforeEpoch`; uniqueness is never affected by clock movement
  because the registry tracks sequences per timestamp.

## Errors

`New`/`NewParser` validation errors all wrap `ErrInvalidConfig`: `ErrInvalidBitFormat`,
`ErrEpochInFuture`, `ErrInvalidBlockSize`, `ErrInvalidThreshold`. `New` also returns
`ErrNilRegistry`.

`Generate` may return `ErrClockBeforeEpoch`, `ErrTimestampOverflow`, `ErrSequenceOverflow`
(too many IDs in one second), or `ErrInvalidSequence`, in addition to any error surfaced by
the registry. Use `errors.Is` to match them.

## Concurrency

A `Node` is safe for concurrent use by multiple goroutines. Background pre-allocation runs
in its own goroutine and synchronizes on the node's lock.
