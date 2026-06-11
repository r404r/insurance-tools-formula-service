package api

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rawIPKey is the context key used by PreserveRawIP to store the original
// TCP connection IP before chi's RealIP middleware may overwrite r.RemoteAddr
// with a header value.
type rawIPKey struct{}

// PreserveRawIP is a middleware that must be registered BEFORE chi's RealIP
// middleware. It saves the direct TCP connection IP into the request context
// so that rate limiters can use it regardless of X-Forwarded-For headers.
//
// Deployment note: add this to the global middleware chain before RealIP:
//
//	r.Use(api.PreserveRawIP)
//	r.Use(chiMiddleware.RealIP)
func PreserveRawIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		ctx := context.WithValue(r.Context(), rawIPKey{}, ip)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractClientIP returns the IP to use for rate limiting.
//
// When trustProxy is false (the default and safe setting for direct-internet
// deployments), it reads the raw TCP connection IP saved by PreserveRawIP,
// completely ignoring X-Forwarded-For / X-Real-IP. This prevents IP spoofing:
// a client cannot rotate IPs by sending forged headers.
//
// When trustProxy is true (set SERVER_TRUST_PROXY=true only when the server
// sits behind a single trusted reverse proxy such as nginx), it reads
// X-Real-IP first, then the leftmost entry of X-Forwarded-For, which the
// trusted proxy sets to the real client IP. WARNING: enabling this while the
// server is directly internet-facing lets any client forge their rate-limit
// identity by sending a spoofed X-Real-IP header.
func extractClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
			return ip
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" {
				return ip
			}
		}
	} else {
		// Use the raw TCP IP saved before chi's RealIP could overwrite RemoteAddr.
		if ip, ok := r.Context().Value(rawIPKey{}).(string); ok && ip != "" {
			return ip
		}
	}
	// Fallback: parse current RemoteAddr (correct in tests and direct connections).
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

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

// IPRateLimit returns middleware that limits requests per client IP.
// reqsPerMinute is the sustained refill rate; burst is the initial token count.
// trustProxy controls how the client IP is resolved — see extractClientIP.
func IPRateLimit(reqsPerMinute float64, burst int, trustProxy bool) func(http.Handler) http.Handler {
	limiter := newIPRateLimiter(reqsPerMinute, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractClientIP(r, trustProxy)
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
