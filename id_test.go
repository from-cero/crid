package crid

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func TestID_Int64(t *testing.T) {
	tests := []struct {
		name string
		id   ID
		want int64
	}{
		{"zero", ID(0), 0},
		{"positive", ID(12345), 12345},
		{"max int63", ID(math.MaxInt64), math.MaxInt64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.Int64(); got != tt.want {
				t.Errorf("Int64() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestID_String(t *testing.T) {
	tests := []struct {
		name string
		id   ID
		want string
	}{
		{"zero", ID(0), "0"},
		{"positive", ID(12345), "12345"},
		{"max int63", ID(math.MaxInt64), "9223372036854775807"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestID_MarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		id   ID
		want string
	}{
		{"zero", ID(0), `"0"`},
		{"positive", ID(12345), `"12345"`},
		{"max int63", ID(math.MaxInt64), `"9223372036854775807"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.id)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestID_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    ID
		wantErr bool
	}{
		{"zero", `"0"`, ID(0), false},
		{"positive", `"12345"`, ID(12345), false},
		{"max int63", `"9223372036854775807"`, ID(math.MaxInt64), false},
		{"not quoted", `12345`, 0, true},
		{"not a number", `"abc"`, 0, true},
		{"overflow", `"9223372036854775808"`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var id ID
			err := json.Unmarshal([]byte(tt.data), &id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Unmarshal() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if id != tt.want {
				t.Errorf("Unmarshal() = %d, want %d", id, tt.want)
			}
		})
	}
}

func TestID_JSONRoundTrip(t *testing.T) {
	want := ID(math.MaxInt64)
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got ID
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got != want {
		t.Errorf("round trip = %d, want %d", got, want)
	}
}

func TestID_UnmarshalJSON_PropagatesError(t *testing.T) {
	var id ID
	err := id.UnmarshalJSON([]byte(`not-quoted`))
	if err == nil {
		t.Fatal("UnmarshalJSON() error = nil, want error")
	}
	// Errors come from strconv; ensure they are surfaced rather than swallowed.
	if errors.Is(err, ErrInvalidConfig) {
		t.Errorf("UnmarshalJSON() returned unexpected sentinel error: %v", err)
	}
}
