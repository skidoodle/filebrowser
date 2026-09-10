package guard

import (
	"sync"
	"time"
)

// limiter is a per-key token bucket.
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   float64
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(rate, burst float64) *limiter {
	return &limiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
	}
}

// setRate updates the token replenishment rate and burst limit.
func (l *limiter) setRate(rate, burst float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rate = rate
	l.burst = burst
}

// allow consumes cost tokens; it reports whether the budget sufficed.
func (l *limiter) allow(key string, cost float64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens = min(l.burst, b.tokens+elapsed*l.rate)
	b.last = now

	if b.tokens < cost {
		return false
	}
	b.tokens -= cost
	return true
}

// allowPartial consumes at most want tokens, capped by availability, as
// long as at least floor tokens are available. It never rejects payloads
// larger than the burst: sustained transfers consume the budget as it
// refills instead of being hard-blocked up front.
func (l *limiter) allowPartial(key string, want, floor float64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens = min(l.burst, b.tokens+elapsed*l.rate)
	b.last = now

	if b.tokens < floor {
		return false
	}
	b.tokens -= min(want, b.tokens)
	return true
}

// cleanup evicts buckets idle for longer than maxIdle.
func (l *limiter) cleanup(maxIdle time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-maxIdle)
	for k, b := range l.buckets {
		if b.last.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}

func (l *limiter) startJanitor(interval, maxIdle time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			l.cleanup(maxIdle)
		}
	}()
}
