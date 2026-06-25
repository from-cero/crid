package crid

import "errors"

var (
	// ErrInvalidConfig is returned when New is called with a config that fails validation.
	ErrInvalidConfig = errors.New("invalid config")

	// ErrInvalidBitFormat is returned when format.timestampBits + format.sequenceBits != 63.
	ErrInvalidBitFormat = errors.New("bit format must sum to 63")

	// ErrEpochInFuture is returned when cfg.epoch is the time
	// after the current system time at the time of Node creation.
	ErrEpochInFuture = errors.New("configured epoch is in the future")

	// ErrInvalidBlockSize is returned when cfg.blockSize is not a positive integer
	// or exceeds 2^format.sequenceBits.
	ErrInvalidBlockSize = errors.New("block size must be in [1, 2^sequenceBits]")

	// ErrInvalidThreshold is returned when cfg.threshold exceeds cfg.blockSize.
	ErrInvalidThreshold = errors.New("threshold must not exceed block size")

	// ErrNilRegistry is returned when New is called with a nil Registry.
	ErrNilRegistry = errors.New("registry cannot be nil")

	// ErrClockBeforeEpoch is returned when the system clock is behind the configured epoch.
	ErrClockBeforeEpoch = errors.New("system clock is before the configured epoch")

	// ErrTimestampOverflow is returned when the current time
	// exceeds the maximum representable timestamp for the given format.
	ErrTimestampOverflow = errors.New("timestamp exceeds maximum for the given format")

	// ErrSequenceOverflow is returned when too many IDs are generated in the same second
	// and the sequence number exceeds the maximum for the given format.
	ErrSequenceOverflow = errors.New("too many IDs generated in the same second")

	// ErrInvalidSequence is returned when the sequence number acquired from the registry
	// is out of range for the given format.sequenceBits.
	ErrInvalidSequence = errors.New("invalid sequence number for given format")
)
