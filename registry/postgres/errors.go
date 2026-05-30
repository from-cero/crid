package postgres

import "errors"

var (
	// ErrNilQuerier is returned when New is called with a nil Querier.
	ErrNilQuerier = errors.New("querier cannot be nil")

	// ErrInvalidTable is returned when New is called with a table name that is not a
	// valid SQL identifier (optionally schema-qualified).
	ErrInvalidTable = errors.New("table must be a valid SQL identifier, optionally schema-qualified")

	// ErrAllocate is returned when the registry fails to reserve a block from Postgres.
	ErrAllocate = errors.New("allocate block failed")

	// ErrEnsureSchema is returned when EnsureSchema fails to create the table.
	ErrEnsureSchema = errors.New("ensure schema failed")
)
