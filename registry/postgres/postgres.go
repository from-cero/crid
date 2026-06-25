// Package postgres provides a durable, multi-process registry.Registry backed by
// PostgreSQL. Unlike the in-memory registry, allocations are persisted, so a sequence
// block is never handed out twice even across process restarts, and uniqueness holds
// for every node sharing the same database and table.
//
// The registry talks to Postgres through the pgx driver. Construct it with a
// *pgxpool.Pool (or any pgx connection or transaction that satisfies DB):
//
//	pool, err := pgxpool.New(ctx, dsn)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer pool.Close()
//
//	reg, err := postgres.New(pool)
//	if err != nil {
//		log.Fatal(err)
//	}
//	// Create the table once at startup (or manage it with migrations instead).
//	if err := reg.EnsureSchema(ctx); err != nil {
//		log.Fatal(err)
//	}
//
//	node, err := crid.New(reg)
//
// Each Allocate is a single atomic UPSERT, so concurrent callers - within one process
// or across many - always receive non-overlapping blocks for a given timestamp.
package postgres

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DB is the subset of the pgx API the registry depends on. It is satisfied by
// *pgxpool.Pool, *pgx.Conn, and pgx.Tx, so the registry can run on a connection pool,
// a single connection, or inside a caller-managed transaction.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// defaultTable is the table the registry reads and writes when WithTable is not given.
const defaultTable = "crid_allocations"

// validTable matches a bare or single-schema-qualified SQL identifier. The table name is
// interpolated into queries (it cannot be a bind parameter), so it is restricted to safe
// characters to avoid SQL injection.
var validTable = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

// Registry is a registry.Registry backed by Postgres. It is safe for concurrent use by
// multiple goroutines and across multiple processes sharing the same database and table.
type Registry struct {
	db    DB
	table string
}

// Option configures a Registry at creation time.
type Option func(*Registry)

// WithTable sets the table the registry reads and writes. The name must be a valid SQL
// identifier, optionally schema-qualified (for example "app.crid_allocations"). The
// default is "crid_allocations".
func WithTable(name string) Option { return func(r *Registry) { r.table = name } }

// New returns a Registry that allocates sequence blocks from db, which is typically a
// *pgxpool.Pool. The returned Registry does not create its table; call EnsureSchema once
// at startup, or create the table out of band (see EnsureSchema for the schema).
func New(db DB, opts ...Option) (*Registry, error) {
	r := &Registry{db: db, table: defaultTable}
	for _, opt := range opts {
		opt(r)
	}

	if db == nil {
		return nil, ErrNilDB
	}
	if !validTable.MatchString(r.table) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidTable, r.table)
	}

	return r, nil
}

// EnsureSchema creates the registry's table if it does not already exist.
// It is a convenience for development and simple deployments.
// Production schemas are usually managed by migrations.
func (r *Registry) EnsureSchema(ctx context.Context) error {
	sql := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (ts BIGINT PRIMARY KEY, next_seq BIGINT NOT NULL)`,
		r.table,
	)
	if _, err := r.db.Exec(ctx, sql); err != nil {
		return fmt.Errorf("%w: %w", ErrEnsureSchema, err)
	}
	return nil
}

// VerifySchema reports whether the registry's table already exists. Use it to fail fast
// at startup when the schema is managed out of band (by migrations) rather than created
// with EnsureSchema. The table name is resolved against the connection's search_path the
// same way Allocate resolves it, so a bare name and a schema-qualified name behave
// consistently across both calls.
func (r *Registry) VerifySchema(ctx context.Context) (bool, error) {
	const sql = `SELECT EXISTS (
		SELECT 1 FROM pg_class c
		WHERE c.oid = to_regclass($1) AND c.relkind IN ('r', 'p')
	)`
	var exists bool
	if err := r.db.QueryRow(ctx, sql, r.table).Scan(&exists); err != nil {
		return false, fmt.Errorf("%w: %w", ErrVerifySchema, err)
	}
	return exists, nil
}

// EvictBefore deletes all allocation rows for timestamps strictly before cutoff.
// cutoff is seconds since the Unix epoch, the same unit as the ts column.
// Rows for timestamps still in use must not be evicted; only call this with a cutoff
// safely in the past (for example, one hour ago).
// It is safe to call concurrently with Allocate.
func (r *Registry) EvictBefore(ctx context.Context, cutoff int64) error {
	sql := fmt.Sprintf(`DELETE FROM %s WHERE ts < $1`, r.table)
	if _, err := r.db.Exec(ctx, sql, cutoff); err != nil {
		return fmt.Errorf("%w: %w", ErrEvict, err)
	}
	return nil
}

// Allocate reserves blockSize sequence numbers for timestamp and returns the starting
// value of the reserved block. It runs a single atomic UPSERT, so successive and
// concurrent calls for the same timestamp - across every process sharing the table -
// return non-overlapping blocks.
func (r *Registry) Allocate(ctx context.Context, timestamp, blockSize int64) (int64, error) {
	sql := fmt.Sprintf(
		`INSERT INTO %s AS t (ts, next_seq) VALUES ($1, $2)
		ON CONFLICT (ts) DO UPDATE SET next_seq = t.next_seq + $2
		RETURNING t.next_seq`, r.table,
	)
	var next int64
	if err := r.db.QueryRow(ctx, sql, timestamp, blockSize).Scan(&next); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrAllocate, err)
	}
	// next is the post-increment counter; the block we just reserved starts blockSize back.
	return next - blockSize, nil
}
