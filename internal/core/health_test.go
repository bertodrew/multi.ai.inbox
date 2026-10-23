package core

import (
	"strings"
	"testing"
)

func TestHealthBreaker(t *testing.T) {
	for _, c := range []struct {
		sends, hard int
		want        bool
	}{
		{19, 19, false}, // below the minimum volume nothing trips
		{20, 1, false},  // 5% is the limit, not above it
		{20, 2, true},
		{100, 5, false},
		{100, 6, true},
	} {
		if got := (healthStats{sends: c.sends, hard: c.hard}).tripped(); got != c.want {
			t.Errorf("sends %d hard %d: tripped %v, want %v", c.sends, c.hard, got, c.want)
		}
	}
	if r := (healthStats{sends: 20, hard: 3}).reason(); !strings.Contains(r, "15.0%") || !strings.Contains(r, "last 24h") {
		t.Errorf("reason = %q", r)
	}
}

func TestComplaintBreaker(t *testing.T) {
	for _, c := range []struct {
		sends, complaints int
		want              bool
	}{
		{0, 2, false},   // small sender: fewer than 3 complaints
		{50, 3, true},   // small sender: 3 complaints in the window
		{999, 2, false}, // still small
		{1000, 1, false},
		{1000, 2, true}, // 0.2% is above 0.1%
		{5000, 5, false},
		{5000, 6, true},
	} {
		if got := (healthStats{sends: c.sends, complaints: c.complaints}).tripped(); got != c.want {
			t.Errorf("sends %d complaints %d: tripped %v, want %v", c.sends, c.complaints, got, c.want)
		}
	}
	if r := (healthStats{sends: 1000, complaints: 2}).reason(); !strings.Contains(r, "complaint rate 0.20%") || !strings.Contains(r, "0.1%") {
		t.Errorf("rate reason = %q", r)
	}
	if r := (healthStats{sends: 40, complaints: 3}).reason(); !strings.Contains(r, "3 spam complaints in the last 24h") {
		t.Errorf("small sender reason = %q", r)
	}
	// Bounces win the reason when both trip.
	if r := (healthStats{sends: 40, hard: 10, complaints: 3}).reason(); !strings.Contains(r, "hard bounce rate") {
		t.Errorf("combined reason = %q", r)
	}
}
