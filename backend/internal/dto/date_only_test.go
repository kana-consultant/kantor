package dto

import (
	"testing"
	"time"
)

func TestParseDateOnly(t *testing.T) {
	t.Parallel()

	got, err := ParseDateOnly(DateOnly("2026-04-12"))
	if err != nil {
		t.Fatalf("ParseDateOnly returned error: %v", err)
	}

	want := time.Date(2026, time.April, 12, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("ParseDateOnly() = %v, want %v", got, want)
	}
}

func TestParseDateOnlyRFC3339KeepsCalendarDate(t *testing.T) {
	t.Parallel()

	got, err := ParseDateOnly(DateOnly("2026-04-12T23:15:00+09:00"))
	if err != nil {
		t.Fatalf("ParseDateOnly returned error: %v", err)
	}

	want := time.Date(2026, time.April, 12, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("ParseDateOnly(RFC3339) = %v, want %v", got, want)
	}
}

func TestParseDateOnlyRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	if _, err := ParseDateOnly(DateOnly("")); err == nil {
		t.Fatal("expected empty date to return an error")
	}
	if _, err := ParseDateOnly(DateOnly("not-a-date")); err == nil {
		t.Fatal("expected invalid date to return an error")
	}
}
