package httpapi

import (
	"sync"
	"time"
)

// rateLimiter is an in-process token bucket. The current Compose deployment
// runs one API instance; a multi-instance deployment also needs an edge or
// shared-store limit so callers cannot multiply their allowance per replica.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]rateBucket
	calls   uint64
}

type rateBucket struct {
	tokens   float64
	updated  time.Time
	lastSeen time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: make(map[string]rateBucket)}
}

func (l *rateLimiter) allow(key string, perSecond float64, burst int, now time.Time) bool {
	if l == nil || key == "" || perSecond <= 0 || burst < 1 {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, ok := l.buckets[key]
	if !ok {
		bucket = rateBucket{tokens: float64(burst), updated: now, lastSeen: now}
	}
	if elapsed := now.Sub(bucket.updated).Seconds(); elapsed > 0 {
		bucket.tokens += elapsed * perSecond
		if bucket.tokens > float64(burst) {
			bucket.tokens = float64(burst)
		}
		bucket.updated = now
	}
	bucket.lastSeen = now
	allowed := bucket.tokens >= 1
	if allowed {
		bucket.tokens--
	}
	l.buckets[key] = bucket
	l.calls++
	if l.calls%256 == 0 {
		cutoff := now.Add(-10 * time.Minute)
		for candidate, entry := range l.buckets {
			if entry.lastSeen.Before(cutoff) {
				delete(l.buckets, candidate)
			}
		}
	}
	return allowed
}
