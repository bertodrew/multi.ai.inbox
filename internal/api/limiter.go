package api

import (
	"sync"
	"time"
)

// Limiter is a per organization token bucket kept in memory. One instance
// per API process; with several replicas each enforces its own share.
type Limiter struct {
	Rate  float64 // tokens per second
	Burst float64

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func NewLimiter(perSecond, burst float64) *Limiter {
	return &Limiter{Rate: perSecond, Burst: burst, buckets: map[string]*bucket{}}
}

func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) > 100_000 {
			l.buckets = map[string]*bucket{} // crude bound on memory
		}
		b = &bucket{tokens: l.Burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.Burst, b.tokens+now.Sub(b.last).Seconds()*l.Rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Peek reports whether key has a token left without consuming it.
func (l *Limiter) Peek(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		return true
	}
	return min(l.Burst, b.tokens+now.Sub(b.last).Seconds()*l.Rate) >= 1
}
