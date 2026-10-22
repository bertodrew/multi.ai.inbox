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
