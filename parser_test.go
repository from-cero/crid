package crid

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/from-cero/crid/registry/memory"
)

func TestNewParser_InvalidFormat(t *testing.T) {
	p, err := NewParser(WithFormat(WithTimestampBits(1), WithSequenceBits(1)))
	if !errors.Is(err, ErrInvalidBitFormat) {
		t.Errorf("NewParser() = %v, want ErrInvalidBitFormat", err)
	}
	if p != nil {
		t.Error("NewParser() returned non-nil parser on error")
	}
}

func TestNewParser_Valid(t *testing.T) {
	p, err := NewParser()
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}
	if p == nil {
		t.Fatal("NewParser() returned nil parser")
	}
}

func TestParser_Parse_KnownID(t *testing.T) {
	// Use default format: shift_ts=32
	epoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	wantTS := int64(500) // 500s since epoch
	wantSeq := int64(7)

	// Pack: ts << 32 | seq
	packed := (wantTS << 32) | wantSeq
	id := ID(packed)

	p, err := NewParser(WithEpoch(epoch))
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}

	parsed := p.Parse(id)
	wantTime := epoch.Add(time.Duration(wantTS) * time.Second)

	if !parsed.Timestamp.Equal(wantTime) {
		t.Errorf("Timestamp = %v, want %v", parsed.Timestamp, wantTime)
	}
	if parsed.Sequence != wantSeq {
		t.Errorf("Sequence = %d, want %d", parsed.Sequence, wantSeq)
	}
}

func TestParser_Parse_RoundTrip(t *testing.T) {
	n, err := New(memory.New())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	before := time.Now()
	id, err := n.Generate(context.Background())
	after := time.Now()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	p, err := NewParser()
	if err != nil {
		t.Fatalf("NewParser() error = %v", err)
	}

	parsed := p.Parse(id)

	// Allow 1s slack for timestamp comparison
	if parsed.Timestamp.Before(before.Add(-time.Second)) || parsed.Timestamp.After(after.Add(time.Second)) {
		t.Errorf("Timestamp = %v out of expected range [%v, %v]", parsed.Timestamp, before, after)
	}
	if parsed.Sequence < 0 {
		t.Errorf("Sequence = %d, want >= 0", parsed.Sequence)
	}
}
