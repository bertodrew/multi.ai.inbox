package api

import "testing"

func TestLimiterBurstThenBlocks(t *testing.T) {
	l := NewLimiter(1, 3)
	for i := range 3 {
		if !l.Allow("org") {
			t.Fatalf("request %d blocked within burst", i)
		}
	}
	if l.Allow("org") {
		t.Fatal("request beyond burst allowed")
	}
	if !l.Allow("other") {
		t.Fatal("organizations must not share a bucket")
	}
}
