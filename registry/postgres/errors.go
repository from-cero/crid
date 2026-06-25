package postgres

import "errors"

var (
	// ErrNilDB is returned when New is called with a nil DB.
	ErrNilDB = errors.New("db cannot be nil")

	// ErrInvalidTable is returned when New is called with a table name that is not a
	// valid SQL identifier (optionally schema-qualified).
	ErrInvalidTable = errors.New("table must be a valid SQL identifier, optionally schema-qualified")

	// ErrAllocate is returned when the registry fails to reserve a block from Postgres.
	ErrAllocate = errors.New("allocate block failed")

	// ErrEnsureSchema is returned when EnsureSchema fails to create the table.
	ErrEnsureSchema = errors.New("ensure schema failed")

	// ErrVerifySchema is returned when VerifySchema fails to check whether the table exists.
	ErrVerifySchema = errors.New("verify schema failed")

	// ErrEvict is returned when EvictBefore fails to delete rows from Postgres.
	ErrEvict = errors.New("evict failed")
)
