package guard

import (
	"sync"
	"time"
)

// bans tracks escalating temporary bans per client key.
type bans struct {
	mu       sync.Mutex
	strikes  map[string]int
	lastSeen map[string]time.Time
	until    map[string]time.Time
}

const (
	strikeWindow    = 15 * time.Minute
	strikeThreshold = 5
	banBase         = 15 * time.Minute
	banMax          = 24 * time.Hour
)

func newBans() *bans {
	return &bans{
		strikes:  make(map[string]int),
		lastSeen: make(map[string]time.Time),
		until:    make(map[string]time.Time),
	}
}

// Banned reports whether the key is currently banned and for how long.
func (b *bans) Banned(key string) (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	until, ok := b.until[key]
	if !ok {
		return false, 0
	}
	remaining := time.Until(until)
	if remaining <= 0 {
		delete(b.until, key) // served its time; strikes persist
		return false, 0
	}
	return true, remaining
}

// Strike records abuse and escalates to a ban once the threshold is hit
// within the strike window. Repeated offenses double the ban length.
func (b *bans) Strike(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if last, ok := b.lastSeen[key]; !ok || now.Sub(last) > strikeWindow {
		b.strikes[key] = 0
	}
	b.lastSeen[key] = now
	b.strikes[key]++

	if b.strikes[key] >= strikeThreshold {
		excess := b.strikes[key] - strikeThreshold
		dur := banBase << min(excess, 20) // 15m, 30m, 1h, … capped at 24h
		b.until[key] = now.Add(min(dur, banMax))
	}
}

// Ban imposes an immediate ban of the given duration.
func (b *bans) Ban(key string, dur time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.until[key] = time.Now().Add(dur)
}

// startJanitor periodically evicts stale strike records.
func (b *bans) startJanitor(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			b.mu.Lock()
			cutoff := time.Now().Add(-strikeWindow)
			for k, last := range b.lastSeen {
				if last.Before(cutoff) {
					delete(b.lastSeen, k)
					delete(b.strikes, k)
				}
			}
			for k, until := range b.until {
				if until.Before(time.Now()) {
					delete(b.until, k)
				}
			}
			b.mu.Unlock()
		}
	}()
}
