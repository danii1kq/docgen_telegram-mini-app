package api

import (
	"sync"
	"time"
)

// limiter is a small fixed-window rate limiter keyed by "<bucket>|<ip>".
// It is deliberately in-memory: one instance is the documented deployment
// shape for this service (see README "Limits"). Behind a load balancer with
// several API replicas, the window is per replica.
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*window
}

type window struct {
	resetAt time.Time
	count   int
}

func newLimiter() *limiter {
	return &limiter{buckets: map[string]*window{}}
}

// allow reports whether the call fits into the window and returns the seconds
// the client should wait otherwise.
func (l *limiter) allow(key string, limit int, size time.Duration) (bool, int) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	// Opportunistic cleanup keeps the map small without a background goroutine.
	if len(l.buckets) > 10_000 {
		for k, w := range l.buckets {
			if now.After(w.resetAt) {
				delete(l.buckets, k)
			}
		}
	}

	entry, ok := l.buckets[key]
	if !ok || now.After(entry.resetAt) {
		l.buckets[key] = &window{resetAt: now.Add(size), count: 1}
		return true, 0
	}
	if entry.count >= limit {
		retry := int(entry.resetAt.Sub(now).Seconds())
		if retry < 1 {
			retry = 1
		}
		return false, retry
	}
	entry.count++
	return true, 0
}
