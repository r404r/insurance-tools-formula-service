package api

import (
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger returns middleware that logs each request's method, path, status code,
// and duration using the provided zerolog logger.
func Logger(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			logger.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", sw.status).
				Dur("duration", time.Since(start)).
				Msg("request completed")
		})
	}
}

// statusWriter wraps http.ResponseWriter to capture the response status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// Recovery returns middleware that recovers from panics, logs the stack trace,
// and responds with a 500 Internal Server Error.
func Recovery() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					stack := debug.Stack()
					log.Error().
						Interface("panic", rec).
						Bytes("stack", stack).
						Msg("recovered from panic")
					http.Error(w, `{"error":"internal server error","code":500}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// DynamicConcurrencyLimiter is a semaphore-based concurrency cap whose limit
// can be updated at runtime without restarting the server.
//
// In-flight tracking: an atomic counter records how many requests are currently
// holding a slot. When SetLimit is called, the new semaphore is pre-filled with
// min(inflight, newLimit) tokens so that the available capacity equals
// max(0, newLimit - inflight), preventing a fresh-channel bypass where a
// limit change could allow more concurrent requests than intended.
type DynamicConcurrencyLimiter struct {
	mu       sync.RWMutex
	sem      chan struct{}
	limit    int
	inflight atomic.Int64
}

// NewDynamicConcurrencyLimiter creates a limiter with an initial cap of n.
// n ≤ 0 means unlimited.
func NewDynamicConcurrencyLimiter(n int) *DynamicConcurrencyLimiter {
	d := &DynamicConcurrencyLimiter{limit: n}
	if n > 0 {
		d.sem = make(chan struct{}, n)
	}
	return d
}

// SetLimit swaps the semaphore to capacity n, accounting for currently in-flight
// requests so the effective available slots stay correct across the transition.
// n ≤ 0 disables the limit.
func (d *DynamicConcurrencyLimiter) SetLimit(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.limit = n
	if n <= 0 {
		d.sem = nil
		return
	}
	newSem := make(chan struct{}, n)
	// Pre-fill for currently in-flight requests so the new cap is accurate.
	inFlight := int(d.inflight.Load())
	preFill := inFlight
	if preFill > n {
		preFill = n
	}
	for i := 0; i < preFill; i++ {
		newSem <- struct{}{}
	}
	d.sem = newSem
}

// Limit returns the current cap (0 means unlimited).
func (d *DynamicConcurrencyLimiter) Limit() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.limit
}

// Middleware returns an http.Handler middleware that enforces the current limit.
// When the cap is reached the handler returns 503 + Retry-After: 1 immediately.
func (d *DynamicConcurrencyLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			d.mu.RLock()
			sem := d.sem
			d.mu.RUnlock()

			if sem == nil {
				d.inflight.Add(1)
				defer d.inflight.Add(-1)
				next.ServeHTTP(w, r)
				return
			}

			select {
			case sem <- struct{}{}:
				d.inflight.Add(1)
				defer func() {
					<-sem
					d.inflight.Add(-1)
				}()
				next.ServeHTTP(w, r)
			default:
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{
					Error: "server busy: too many concurrent calculations, please retry",
					Code:  http.StatusServiceUnavailable,
				})
			}
		})
	}
}

// CORS returns middleware that sets Cross-Origin Resource Sharing headers for
// the given list of allowed origins.
func CORS(origins []string) func(http.Handler) http.Handler {
	c := cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           300,
	})
	return c.Handler
}
