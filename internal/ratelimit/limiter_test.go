package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterFixedWindow(t *testing.T) {
	now := time.Date(2026, 7, 27, 4, 0, 0, 0, time.UTC)
	limiter := New(2, time.Minute, 16, func() time.Time { return now })
	if !limiter.Allow("192.0.2.1") || !limiter.Allow("192.0.2.1") {
		t.Fatal("first two requests should be allowed")
	}
	if limiter.Allow("192.0.2.1") {
		t.Fatal("third request should be denied")
	}
	now = now.Add(time.Minute)
	if !limiter.Allow("192.0.2.1") {
		t.Fatal("request after window reset should be allowed")
	}
}

func TestLimiterBoundsTrackedIPs(t *testing.T) {
	now := time.Date(2026, 7, 27, 4, 0, 0, 0, time.UTC)
	limiter := New(1, time.Minute, 3, func() time.Time { return now })
	for _, ip := range []string{"192.0.2.1", "192.0.2.2", "192.0.2.3", "192.0.2.4"} {
		if !limiter.Allow(ip) {
			t.Fatalf("first request for %s should be allowed", ip)
		}
		now = now.Add(time.Second)
	}
	if limiter.Len() > 3 {
		t.Fatalf("tracked IPs = %d, want <= 3", limiter.Len())
	}
}
