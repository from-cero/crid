package crid

import (
	"errors"
	"testing"
)

func TestApplyFormatOptions_Defaults(t *testing.T) {
	f := applyFormatOptions(nil)
	if f != defaultFormat {
		t.Errorf("applyFormatOptions(nil) = %+v, want %+v", f, defaultFormat)
	}
}

func TestApplyFormatOptions_Overrides(t *testing.T) {
	f := applyFormatOptions(
		[]FormatOption{
			WithTimestampBits(41),
			WithSequenceBits(22),
		},
	)
	if f.timestampBits != 41 {
		t.Errorf("timestampBits = %d, want 41", f.timestampBits)
	}
	if f.sequenceBits != 22 {
		t.Errorf("sequenceBits = %d, want 22", f.sequenceBits)
	}
}

func TestFormat_Validate(t *testing.T) {
	tests := []struct {
		name          string
		timestampBits uint8
		sequenceBits  uint8
		wantErr       bool
	}{
		{"default", 31, 32, false},
		{"alternative split sums to 63", 41, 22, false},
		{"sum too small", 1, 1, true},
		{"sum too large", 40, 40, true},
		{"sum 62", 30, 32, true},
		{"sum 64", 32, 32, true},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				f := format{timestampBits: tt.timestampBits, sequenceBits: tt.sequenceBits}
				err := f.validate()
				if tt.wantErr {
					if !errors.Is(err, ErrInvalidBitFormat) {
						t.Errorf("validate() = %v, want ErrInvalidBitFormat", err)
					}
					return
				}
				if err != nil {
					t.Errorf("validate() = %v, want nil", err)
				}
			},
		)
	}
}

func TestFormat_CompileFormat(t *testing.T) {
	tests := []struct {
		name        string
		f           format
		wantShift   uint8
		wantMaxTime int64
		wantMaxSeq  int64
	}{
		{
			name:        "default 31/32",
			f:           format{timestampBits: 31, sequenceBits: 32},
			wantShift:   32,
			wantMaxTime: (1 << 31) - 1,
			wantMaxSeq:  (1 << 32) - 1,
		},
		{
			name:        "41/22 split",
			f:           format{timestampBits: 41, sequenceBits: 22},
			wantShift:   22,
			wantMaxTime: (1 << 41) - 1,
			wantMaxSeq:  (1 << 22) - 1,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				c := tt.f.compileFormat()
				if c.shiftTimestamp != tt.wantShift {
					t.Errorf("shiftTimestamp = %d, want %d", c.shiftTimestamp, tt.wantShift)
				}
				if c.maxTimestamp != tt.wantMaxTime {
					t.Errorf("maxTimestamp = %d, want %d", c.maxTimestamp, tt.wantMaxTime)
				}
				if c.maxSequence != tt.wantMaxSeq {
					t.Errorf("maxSeq = %d, want %d", c.maxSequence, tt.wantMaxSeq)
				}
			},
		)
	}
}
