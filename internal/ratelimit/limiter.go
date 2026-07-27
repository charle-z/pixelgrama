package ratelimit

import (
	"sync"
	"time"
)

type entry struct {
	count       int
	windowStart time.Time
	lastSeen    time.Time
}

type Limiter struct {
	mu         sync.Mutex
	limit      int
	window     time.Duration
	maxEntries int
	now        func() time.Time
	entries    map[string]entry
}

func New(limit int, window time.Duration, maxEntries int, now func() time.Time) *Limiter {
	if limit < 1 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	if maxEntries < 1 {
		maxEntries = 1
	}
	if now == nil {
		now = time.Now
	}
	return &Limiter{
		limit:      limit,
		window:     window,
		maxEntries: maxEntries,
		now:        now,
		entries:    make(map[string]entry),
	}
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.removeExpired(now)
	current, exists := l.entries[key]
	if exists && now.Sub(current.windowStart) < l.window {
		current.lastSeen = now
		if current.count >= l.limit {
			l.entries[key] = current
			return false
		}
		current.count++
		l.entries[key] = current
		return true
	}

	if !exists && len(l.entries) >= l.maxEntries {
		l.removeOldest()
	}
	l.entries[key] = entry{count: 1, windowStart: now, lastSeen: now}
	return true
}

func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

func (l *Limiter) removeExpired(now time.Time) {
	for key, value := range l.entries {
		if now.Sub(value.windowStart) >= l.window {
			delete(l.entries, key)
		}
	}
}

func (l *Limiter) removeOldest() {
	var oldestKey string
	var oldest time.Time
	first := true
	for key, value := range l.entries {
		if first || value.lastSeen.Before(oldest) {
			oldestKey = key
			oldest = value.lastSeen
			first = false
		}
	}
	if !first {
		delete(l.entries, oldestKey)
	}
}
