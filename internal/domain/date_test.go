package domain

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestDate_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		date     Date
		wantJSON string
	}{
		{"known date", NewDate(2026, time.October, 18), `"2026-10-18"`},
		{"zero is null", Date{}, `null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.date)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tt.wantJSON {
				t.Fatalf("Marshal = %s, want %s", b, tt.wantJSON)
			}
			var back Date
			if err := json.Unmarshal(b, &back); err != nil {
				t.Fatal(err)
			}
			if back != tt.date {
				t.Errorf("round trip = %v, want %v", back, tt.date)
			}
		})
	}
}

func TestDate_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"empty string is zero", `""`, "", false},
		{"invalid date", `"18/10/2026"`, "", true},
		{"not a string", `123`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Date
			err := json.Unmarshal([]byte(tt.in), &d)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if d.String() != tt.want {
				t.Errorf("String() = %q, want %q", d.String(), tt.want)
			}
		})
	}
}

func TestParseDate_InvalidIsErrInvalidInput(t *testing.T) {
	if _, err := ParseDate("2026-13-01"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestFixedClock_ReturnsUTC(t *testing.T) {
	loc := time.FixedZone("BRT", -3*3600)
	c := FixedClock{T: time.Date(2026, 10, 19, 9, 0, 0, 0, loc)}
	if got := c.Now(); got.Location() != time.UTC || got.Hour() != 12 {
		t.Errorf("Now() = %v, want 12:00 UTC", got)
	}
	if (SystemClock{}).Now().Location() != time.UTC {
		t.Error("SystemClock must return UTC")
	}
}

func TestDate_RefusesToWriteAYearItCannotRead(t *testing.T) {
	for _, y := range []int{-191651464, 10000} {
		if b, err := json.Marshal(NewDate(y, time.May, 1)); err == nil {
			t.Errorf("year %d encoded as %s", y, b)
		}
	}
	if NewDate(1989, 12, 31).Plausible() || !NewDate(2026, 10, 20).Plausible() || (Date{}).Plausible() {
		t.Error("Plausible")
	}
}
