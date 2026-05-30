package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/from-cero/crid"
)

// --- unit tests (no database) -------------------------------------------------

// valRow scans a single int64, modelling the RETURNING value of the UPSERT.
type valRow struct{ v int64 }

func (r valRow) Scan(dest ...any) error {
	*dest[0].(*int64) = r.v
	return nil
}

// errRow fails on Scan, modelling a query error.
type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

// fakeDB is a stand-in for *pgxpool.Pool. It simulates the per-timestamp counter the
// real UPSERT maintains so the Go-side arithmetic and argument passing can be tested
// without a database. Real atomicity is covered by the integration test below.
type fakeDB struct {
	mu       sync.Mutex
	next     map[int64]int64
	lastSQL  string
	lastArgs []any
	execSQL  string
	queryErr error
	execErr  error
}

func newFakeDB() *fakeDB { return &fakeDB{next: make(map[int64]int64)} }

func (f *fakeDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastSQL = sql
	f.lastArgs = args
	if f.queryErr != nil {
		return errRow{f.queryErr}
	}
	ts := args[0].(int64)
	blockSize := args[1].(int64)
	f.next[ts] += blockSize
	return valRow{f.next[ts]}
}

func (f *fakeDB) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execSQL = sql
	return pgconn.CommandTag{}, f.execErr
}

func TestNew_Validation(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, ErrNilQuerier) {
		t.Errorf("New(nil) error = %v, want ErrNilQuerier", err)
	}

	bad := []string{"", "1abc", "no-dash", "two.dots.here", "drop;table", "a b", "tbl$"}
	for _, name := range bad {
		if _, err := New(newFakeDB(), WithTable(name)); !errors.Is(err, ErrInvalidTable) {
			t.Errorf("New(WithTable(%q)) error = %v, want ErrInvalidTable", name, err)
		}
	}

	good := []string{"crid_allocations", "_t", "app.crid_allocations", "S1.T2"}
	for _, name := range good {
		if _, err := New(newFakeDB(), WithTable(name)); err != nil {
			t.Errorf("New(WithTable(%q)) error = %v, want nil", name, err)
		}
	}
}

func TestAllocate_ArgsAndArithmetic(t *testing.T) {
	db := newFakeDB()
	reg, err := New(db)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx := context.Background()
	const ts, blockSize = int64(42), int64(100)

	start1, err := reg.Allocate(ctx, ts, blockSize)
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	if start1 != 0 {
		t.Errorf("first start = %d, want 0", start1)
	}

	start2, err := reg.Allocate(ctx, ts, blockSize)
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	if start2 != blockSize {
		t.Errorf("second start = %d, want %d", start2, blockSize)
	}

	// A different timestamp keeps an independent counter.
	startOther, err := reg.Allocate(ctx, ts+1, blockSize)
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	if startOther != 0 {
		t.Errorf("start for new timestamp = %d, want 0", startOther)
	}

	if len(db.lastArgs) != 2 || db.lastArgs[0] != ts+1 || db.lastArgs[1] != blockSize {
		t.Errorf("query args = %v, want [%d %d]", db.lastArgs, ts+1, blockSize)
	}
}

func TestAllocate_QueryShape(t *testing.T) {
	db := newFakeDB()
	reg, err := New(db, WithTable("app.crid_allocations"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := reg.Allocate(context.Background(), 1, 1); err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}

	for _, want := range []string{"app.crid_allocations", "ON CONFLICT (ts)", "RETURNING"} {
		if !strings.Contains(db.lastSQL, want) {
			t.Errorf("allocate SQL %q does not contain %q", db.lastSQL, want)
		}
	}
}

func TestAllocate_Error(t *testing.T) {
	sentinel := errors.New("boom")
	db := newFakeDB()
	db.queryErr = sentinel
	reg, err := New(db)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = reg.Allocate(context.Background(), 1, 1)
	if !errors.Is(err, ErrAllocate) {
		t.Errorf("Allocate() error = %v, want wrapping ErrAllocate", err)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Allocate() error = %v, want wrapping underlying error", err)
	}
}

func TestEnsureSchema(t *testing.T) {
	db := newFakeDB()
	reg, err := New(db, WithTable("app.crid_allocations"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := reg.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	for _, want := range []string{"CREATE TABLE IF NOT EXISTS", "app.crid_allocations", "ts BIGINT PRIMARY KEY"} {
		if !strings.Contains(db.execSQL, want) {
			t.Errorf("create SQL %q does not contain %q", db.execSQL, want)
		}
	}

	db.execErr = errors.New("nope")
	if err := reg.EnsureSchema(context.Background()); !errors.Is(err, ErrEnsureSchema) {
		t.Errorf("EnsureSchema() error = %v, want wrapping ErrEnsureSchema", err)
	}
}

// --- integration test (real Postgres) ----------------------------------------

// TestIntegration exercises the registry against a live Postgres instance. It is skipped
// unless CRID_POSTGRES_DSN points at a database, e.g.
//
//	CRID_POSTGRES_DSN=postgres://crid:crid@localhost:5432/crid?sslmode=disable go test ./...
func TestIntegration(t *testing.T) {
	dsn := os.Getenv("CRID_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CRID_POSTGRES_DSN to run Postgres integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer pool.Close()

	const table = "crid_allocations_test"
	reg, err := New(pool, WithTable(table))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := reg.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE "+table); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	t.Cleanup(
		func() {
			_, _ = pool.Exec(context.Background(), "DROP TABLE IF EXISTS "+table)
		},
	)

	t.Run(
		"SequentialNonOverlapping", func(t *testing.T) {
			const ts = 42
			for i, want := range []int64{0, 100, 200} {
				block := int64(100)
				if i == 2 {
					block = 50 // size of the *next* reservation does not change this one's start
				}
				start, err := reg.Allocate(ctx, ts, block)
				if err != nil {
					t.Fatalf("Allocate() error = %v", err)
				}
				if start != want {
					t.Errorf("start[%d] = %d, want %d", i, start, want)
				}
			}
		},
	)

	t.Run(
		"IndependentTimestamps", func(t *testing.T) {
			if _, err := reg.Allocate(ctx, 1, 100); err != nil {
				t.Fatalf("Allocate() error = %v", err)
			}
			start, err := reg.Allocate(ctx, 2, 100)
			if err != nil {
				t.Fatalf("Allocate() error = %v", err)
			}
			if start != 0 {
				t.Errorf("start for new timestamp = %d, want 0", start)
			}
		},
	)

	t.Run(
		"Concurrent", func(t *testing.T) {
			const (
				ts         = 7
				goroutines = 50
				blockSize  = 10
			)
			var (
				mu     sync.Mutex
				starts = make(map[int64]struct{}, goroutines)
				wg     sync.WaitGroup
			)
			wg.Add(goroutines)
			for range goroutines {
				go func() {
					defer wg.Done()
					start, err := reg.Allocate(ctx, ts, blockSize)
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
		},
	)

	t.Run(
		"PersistsAcrossRestart", func(t *testing.T) {
			const ts = 200
			if _, err := reg.Allocate(ctx, ts, 100); err != nil {
				t.Fatalf("Allocate() error = %v", err)
			}
			// A brand new Registry (as after a process restart) must continue the counter
			// stored in Postgres, never reusing the block already handed out.
			reg2, err := New(pool, WithTable(table))
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			start, err := reg2.Allocate(ctx, ts, 100)
			if err != nil {
				t.Fatalf("Allocate() error = %v", err)
			}
			if start != 100 {
				t.Errorf("start after restart = %d, want 100", start)
			}
		},
	)

	t.Run(
		"EndToEndWithNodes", func(t *testing.T) {
			// Two nodes sharing the registry must never collide: uniqueness comes from the
			// registry handing out non-overlapping blocks, not from any per-node identity.
			const (
				nodes      = 2
				perNode    = 5_000
				goroutines = 4
			)
			nodeList := make([]*crid.Node, nodes)
			for i := range nodeList {
				node, err := crid.New(reg)
				if err != nil {
					t.Fatalf("crid.New() error = %v", err)
				}
				nodeList[i] = node
			}

			var (
				mu   sync.Mutex
				seen = make(map[crid.ID]struct{}, nodes*perNode)
				wg   sync.WaitGroup
			)
			for _, node := range nodeList {
				for range goroutines {
					wg.Add(1)
					go func(n *crid.Node) {
						defer wg.Done()
						for range perNode / goroutines {
							id, err := n.Generate(ctx)
							if err != nil {
								t.Errorf("Generate() error = %v", err)
								return
							}
							mu.Lock()
							if _, dup := seen[id]; dup {
								t.Errorf("duplicate ID generated: %s", id)
							}
							seen[id] = struct{}{}
							mu.Unlock()
						}
					}(node)
				}
			}
			wg.Wait()

			if want := nodes * (perNode / goroutines) * goroutines; len(seen) != want {
				t.Errorf("generated %d distinct IDs, want %d", len(seen), want)
			}
		},
	)
}
