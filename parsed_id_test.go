package crid

import (
	"strings"
	"testing"
	"time"
)

func TestParsedID_String(t *testing.T) {
	p := ParsedID{
		Timestamp: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		Sequence:  7,
	}
	s := p.String()

	for _, sub := range []string{"timestamp:", "sequence:"} {
		if !strings.Contains(s, sub) {
			t.Errorf("String() = %q, missing %q", s, sub)
		}
	}
	if !strings.Contains(s, "7") {
		t.Errorf("String() = %q, missing sequence value 7", s)
	}
}
