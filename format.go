package crid

// FormatOption configures the bit layout of generated IDs.
type FormatOption func(*format)

// WithTimestampBits sets the number of bits used for the timestamp component.
func WithTimestampBits(bits uint8) FormatOption { return func(f *format) { f.timestampBits = bits } }

// WithSequenceBits sets the number of bits used for the sequence component.
func WithSequenceBits(bits uint8) FormatOption { return func(f *format) { f.sequenceBits = bits } }

type format struct {
	timestampBits uint8
	sequenceBits  uint8
}

var defaultFormat = format{
	timestampBits: 31, // 68 years of timestamps in second
	sequenceBits:  32, // 4 billion sequence numbers per second
}

func applyFormatOptions(opts []FormatOption) format {
	res := defaultFormat
	for _, opt := range opts {
		opt(&res)
	}
	return res
}

func (f *format) validate() error {
	sum := int(f.timestampBits) + int(f.sequenceBits)
	if sum != 63 {
		return ErrInvalidBitFormat
	}
	return nil
}

type compiledFormat struct {
	shiftTimestamp uint8
	maxTimestamp   int64
	maxSequence    int64
}

func (f *format) compileFormat() *compiledFormat {
	st := f.sequenceBits
	mask := func(bits uint8) int64 {
		return (int64(1) << bits) - 1
	}
	return &compiledFormat{
		shiftTimestamp: st,
		maxTimestamp:   mask(f.timestampBits),
		maxSequence:    mask(f.sequenceBits),
	}
}
