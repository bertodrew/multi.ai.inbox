package core

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

var errTemp = errors.New("421 try again later")

func TestOutboundRetryTemporaryInsideWindow(t *testing.T) {
	created := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	j := OutboundJob{CreatedAt: created, Attempts: 1}
	retry, delay := outboundRetry(j, errTemp, created.Add(time.Minute), DefaultSendMaxAge)
	if !retry || delay != time.Minute {
		t.Fatalf("attempt 1: got retry=%v delay=%v, want retry after 1m", retry, delay)
	}
	j.Attempts = 20
	retry, delay = outboundRetry(j, errTemp, created.Add(48*time.Hour), DefaultSendMaxAge)
	if !retry || delay != 3*time.Hour {
		t.Fatalf("attempt 20: got retry=%v delay=%v, want retry after 3h", retry, delay)
	}
}

func TestOutboundRetryTemporaryAfterWindow(t *testing.T) {
	created := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	j := OutboundJob{CreatedAt: created, Attempts: 40}
	if retry, _ := outboundRetry(j, errTemp, created.Add(DefaultSendMaxAge), DefaultSendMaxAge); retry {
		t.Fatal("message at the max age must fail")
	}
	if retry, _ := outboundRetry(j, errTemp, created.Add(6*24*time.Hour), DefaultSendMaxAge); retry {
		t.Fatal("message past the max age must fail")
	}
	// The last retry is clamped to the end of the window, not scheduled past it.
	retry, delay := outboundRetry(j, errTemp, created.Add(DefaultSendMaxAge-time.Hour), DefaultSendMaxAge)
	if !retry || delay != time.Hour {
		t.Fatalf("near the end: got retry=%v delay=%v, want retry after 1h", retry, delay)
	}
}

func TestOutboundRetryPermanent(t *testing.T) {
	created := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	j := OutboundJob{CreatedAt: created, Attempts: 1}
	err := fmt.Errorf("550 no such user: %w", ErrPermanent)
	if retry, _ := outboundRetry(j, err, created.Add(time.Second), DefaultSendMaxAge); retry {
		t.Fatal("5xx must fail at once")
	}
}

// Simulates a message that fails every attempt: it stays queued until the
// 5 day window ends, retries at least every 3 hours, and then fails.
func TestOutboundRetryScheduleSpansMaxAge(t *testing.T) {
	created := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	now := created
	j := OutboundJob{CreatedAt: created}
	for {
		j.Attempts++
		retry, delay := outboundRetry(j, errTemp, now, DefaultSendMaxAge)
		if !retry {
			break
		}
		if delay <= 0 || delay > 3*time.Hour {
			t.Fatalf("attempt %d: delay %v out of range", j.Attempts, delay)
		}
		// RFC 5321: after the first retries, wait at least 30 minutes (the final
		// retry may be shorter because it is clamped to the window end).
		if j.Attempts > 2 && delay < 30*time.Minute && !now.Add(delay).Equal(created.Add(DefaultSendMaxAge)) {
			t.Fatalf("attempt %d: delay %v below 30m", j.Attempts, delay)
		}
		now = now.Add(delay)
		if j.Attempts > 1000 {
			t.Fatal("schedule never ends")
		}
	}
	if got := now.Sub(created); got != DefaultSendMaxAge {
		t.Fatalf("gave up after %v, want %v", got, DefaultSendMaxAge)
	}
	if j.Attempts < 40 || j.Attempts > 45 {
		t.Fatalf("attempts = %d, want about 42", j.Attempts)
	}
}

func TestSendMaxAgeDefault(t *testing.T) {
	if got := (&Service{}).sendMaxAge(); got != DefaultSendMaxAge {
		t.Fatalf("default = %v", got)
	}
	if got := (&Service{SendMaxAge: time.Hour}).sendMaxAge(); got != time.Hour {
		t.Fatalf("override = %v", got)
	}
}
