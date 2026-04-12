package whatsapp

import (
	"testing"
	"time"
)

func TestCronMatches(t *testing.T) {
	t.Parallel()

	mondayAtEight := time.Date(2026, time.April, 13, 8, 0, 0, 0, time.Local)
	if !cronMatches("0 8 * * 1", mondayAtEight) {
		t.Fatal("expected monday 08:00 to match weekly digest cron")
	}
	if cronMatches("0 9 * * 1", mondayAtEight) {
		t.Fatal("expected monday 08:00 not to match 09:00 cron")
	}
	if !cronMatches("*/15 8-9 * * 1-5", time.Date(2026, time.April, 13, 8, 45, 0, 0, time.Local)) {
		t.Fatal("expected stepped cron expression to match")
	}
}

func TestNextCronOccurrence(t *testing.T) {
	t.Parallel()

	after := time.Date(2026, time.April, 13, 7, 59, 0, 0, time.Local)
	next := nextCronOccurrence("0 8 * * *", after)
	if next == nil {
		t.Fatal("expected next cron occurrence")
	}

	want := time.Date(2026, time.April, 13, 8, 0, 0, 0, time.Local)
	if !next.Equal(want) {
		t.Fatalf("nextCronOccurrence() = %v, want %v", *next, want)
	}
}

func TestNextCronOccurrenceReturnsNilForBlankExpression(t *testing.T) {
	t.Parallel()

	if next := nextCronOccurrence(" ", time.Now()); next != nil {
		t.Fatalf("expected nil for blank cron expression, got %v", *next)
	}
}
