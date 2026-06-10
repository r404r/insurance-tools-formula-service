package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// ipBucket is a per-IP token bucket.
type ipBucket struct {
	tokens   float64
	rate     float64 // tokens refilled per second
	burst    float64
	lastFill time.Time
	mu       sync.Mutex
}

func (b *ipBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastFill).Seconds()
	b.lastFill = now
	b.tokens += elapsed * b.rate
	if b.tokens > b.burst {
		b.tokens = b.burst
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// ipRateLimiter maintains per-IP token buckets and evicts stale entries.
type ipRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*ipBucket
	rate    float64
	burst   float64
}

func newIPRateLimiter(reqsPerMinute float64, burst int) *ipRateLimiter {
	l := &ipRateLimiter{
		buckets: make(map[string]*ipBucket),
		rate:    reqsPerMinute / 60.0,
		burst:   float64(burst),
	}
	go l.cleanup()
	return l
}

func (l *ipRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		l.mu.Lock()
		for ip, b := range l.buckets {
			b.mu.Lock()
			idle := time.Since(b.lastFill) > 10*time.Minute
			b.mu.Unlock()
			if idle {
				delete(l.buckets, ip)
			}
		}
		l.mu.Unlock()
	}
}

func (l *ipRateLimiter) get(ip string) *ipBucket {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[ip]
	if !ok {
		b = &ipBucket{
			tokens:   l.burst,
			rate:     l.rate,
			burst:    l.burst,
			lastFill: time.Now(),
		}
		l.buckets[ip] = b
	}
	return b
}

// IPRateLimit returns middleware that limits requests per IP. reqsPerMinute is
// the sustained rate; burst is the maximum burst size.
func IPRateLimit(reqsPerMinute float64, burst int) func(http.Handler) http.Handler {
	limiter := newIPRateLimiter(reqsPerMinute, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			if !limiter.get(ip).allow() {
				w.Header().Set("Retry-After", "60")
				writeJSON(w, http.StatusTooManyRequests, ErrorResponse{
					Error: "too many requests, please try again later",
					Code:  http.StatusTooManyRequests,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
