package domain_test

import (
	"testing"
	"time"

	"github.com/ynov/equipment-loans/internal/domain"
)

func paris(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestAC3DueAt(t *testing.T) {
	loc := paris(t)
	tests := []struct {
		name       string
		borrowedAt time.Time
		days       int
		want       time.Time
	}{
		{"morning", time.Date(2025, 1, 10, 9, 0, 0, 0, loc), 7, time.Date(2025, 1, 18, 0, 0, 0, 0, loc)},
		{"last second of day", time.Date(2025, 1, 10, 23, 59, 59, 0, loc), 7, time.Date(2025, 1, 18, 0, 0, 0, 0, loc)},
		{"midnight exactly", time.Date(2025, 1, 10, 0, 0, 0, 0, loc), 7, time.Date(2025, 1, 18, 0, 0, 0, 0, loc)},
		{"utc input late evening is next paris day", time.Date(2025, 1, 10, 23, 30, 0, 0, time.UTC), 7, time.Date(2025, 1, 19, 0, 0, 0, 0, loc)},
		{"kit 14 days", time.Date(2025, 1, 10, 9, 0, 0, 0, loc), 14, time.Date(2025, 1, 25, 0, 0, 0, 0, loc)},
		{"spring forward crossing", time.Date(2025, 3, 25, 12, 0, 0, 0, loc), 7, time.Date(2025, 4, 2, 0, 0, 0, 0, loc)},
		{"spring forward before", time.Date(2025, 3, 26, 12, 0, 0, 0, loc), 7, time.Date(2025, 4, 3, 0, 0, 0, 0, loc)},
		{"fall back crossing", time.Date(2025, 10, 22, 12, 0, 0, 0, loc), 7, time.Date(2025, 10, 30, 0, 0, 0, 0, loc)},
		{"borrowed on spring forward day", time.Date(2025, 3, 30, 12, 0, 0, 0, loc), 1, time.Date(2025, 4, 1, 0, 0, 0, 0, loc)},
		{"borrowed on fall back day", time.Date(2025, 10, 26, 12, 0, 0, 0, loc), 1, time.Date(2025, 10, 28, 0, 0, 0, 0, loc)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.DueAt(tc.borrowedAt, tc.days)
			if !got.Equal(tc.want) {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestAC3DueAtUsesParisWallClock(t *testing.T) {
	loc := paris(t)
	got := domain.DueAt(time.Date(2025, 3, 25, 12, 0, 0, 0, loc), 7).In(loc)
	if got.Hour() != 0 || got.Minute() != 0 {
		t.Fatalf("not paris midnight: %s", got)
	}
	if _, off := got.Zone(); off != 7200 {
		t.Fatalf("expected CEST offset, got %d", off)
	}
}

func TestAC3IsOverdueBoundaries(t *testing.T) {
	loc := paris(t)
	due := time.Date(2025, 1, 18, 0, 0, 0, 0, loc)
	loan := domain.Loan{DueAt: due}
	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"eve of last day", time.Date(2025, 1, 16, 12, 0, 0, 0, loc), false},
		{"last day 10h", time.Date(2025, 1, 17, 10, 0, 0, 0, loc), false},
		{"last day 23:59:59", time.Date(2025, 1, 17, 23, 59, 59, 0, loc), false},
		{"now equals due", due, false},
		{"one nanosecond after", due.Add(time.Nanosecond), true},
		{"next day", time.Date(2025, 1, 18, 8, 0, 0, 0, loc), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.IsOverdue(loan, tc.now); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestAC3IsOverdueAcrossDST(t *testing.T) {
	loc := paris(t)
	springDue := domain.DueAt(time.Date(2025, 3, 25, 12, 0, 0, 0, loc), 7)
	fallDue := domain.DueAt(time.Date(2025, 10, 22, 12, 0, 0, 0, loc), 7)
	tests := []struct {
		name string
		due  time.Time
		now  time.Time
		want bool
	}{
		{"spring 23:59:59 local", springDue, time.Date(2025, 4, 1, 23, 59, 59, 0, loc), false},
		{"spring now equals due", springDue, time.Date(2025, 4, 2, 0, 0, 0, 0, loc), false},
		{"spring one ns after", springDue, time.Date(2025, 4, 2, 0, 0, 0, 1, loc), true},
		{"fall 23:59:59 local", fallDue, time.Date(2025, 10, 29, 23, 59, 59, 0, loc), false},
		{"fall now equals due", fallDue, time.Date(2025, 10, 30, 0, 0, 0, 0, loc), false},
		{"fall one ns after", fallDue, time.Date(2025, 10, 30, 0, 0, 0, 1, loc), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.IsOverdue(domain.Loan{DueAt: tc.due}, tc.now); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestAC5ExtendedDueAt(t *testing.T) {
	loc := paris(t)
	tests := []struct {
		name string
		due  time.Time
		days int
		want time.Time
	}{
		{"plain", time.Date(2025, 1, 18, 0, 0, 0, 0, loc), 7, time.Date(2025, 1, 25, 0, 0, 0, 0, loc)},
		{"across spring forward", time.Date(2025, 3, 26, 0, 0, 0, 0, loc), 7, time.Date(2025, 4, 2, 0, 0, 0, 0, loc)},
		{"across fall back", time.Date(2025, 10, 22, 0, 0, 0, 0, loc), 7, time.Date(2025, 10, 29, 0, 0, 0, 0, loc)},
		{"utc representation", time.Date(2025, 3, 26, 0, 0, 0, 0, loc).UTC(), 7, time.Date(2025, 4, 2, 0, 0, 0, 0, loc)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.ExtendedDueAt(tc.due, tc.days)
			if !got.Equal(tc.want) {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
