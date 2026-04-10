package api

import (
	"context"
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
// Generation semantics: every call to SetLimit replaces the internal semaphore
// channel and closes a "generation done" signal to wake any waiters blocked in
// Acquire so they can re-read the new generation and try again. Releases
// captured the specific generation they acquired from, so draining happens on
// the correct channel even if the semaphore has been swapped since.
//
// Shrink semantics are **eventually consistent**: old holders continue to run
// to completion against the old (detached) semaphore, so during the transient
// the effective concurrency can briefly exceed the new cap by up to `oldInflight`.
// No pre-filling is done on the new semaphore because the only way to track
// which holders belong to which generation would leak complexity into every
// Release path. In practice live-shrinks are rare and the transient is bounded
// by the natural completion time of in-flight requests.
type DynamicConcurrencyLimiter struct {
	mu       sync.RWMutex
	sem      chan struct{}
	genDone  chan struct{} // closed when the current generation is retired
	limit    int
	inflight atomic.Int64
}

// NewDynamicConcurrencyLimiter creates a limiter with an initial cap of n.
// n ≤ 0 means unlimited.
func NewDynamicConcurrencyLimiter(n int) *DynamicConcurrencyLimiter {
	d := &DynamicConcurrencyLimiter{limit: n, genDone: make(chan struct{})}
	if n > 0 {
		d.sem = make(chan struct{}, n)
	}
	return d
}

// SetLimit swaps the semaphore to capacity n and retires the previous
// generation so any waiters blocked in Acquire will wake and re-read the
// new channel. Currently in-flight holders continue to drain from the
// previous (now-detached) generation — see the type comment for the
// eventually-consistent shrink semantics.
// n ≤ 0 disables the limit.
func (d *DynamicConcurrencyLimiter) SetLimit(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.limit = n
	// Retire the current generation: close the done channel to kick any
	// Acquire waiters, then allocate a fresh one for the next generation.
	close(d.genDone)
	d.genDone = make(chan struct{})
	if n <= 0 {
		d.sem = nil
		return
	}
	d.sem = make(chan struct{}, n)
}

// Limit returns the current cap (0 means unlimited).
func (d *DynamicConcurrencyLimiter) Limit() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.limit
}

// Acquire blocks until a slot is available in the current semaphore, or the
// context is cancelled. It is intended for in-process consumers such as the
// batch-test handler that run multiple calculations per incoming request and
// must participate in the same global concurrency budget as single-calculation
// requests. Unlike Middleware which returns 503 immediately when the cap is
// reached, Acquire waits — callers are already committed to doing the work.
//
// The returned release function MUST be called exactly once per successful
// Acquire (typically via defer). The closure captures the specific semaphore
// generation that was written to, so if SetLimit swaps the semaphore mid-flight
// Release still drains from the channel the token was written to — the new
// semaphore's available capacity is not clobbered.
//
// If SetLimit retires the generation while Acquire is blocked waiting on the
// old semaphore, Acquire wakes (via the generation-done signal) and re-reads
// d.sem so subsequent attempts honour the new limit.
func (d *DynamicConcurrencyLimiter) Acquire(ctx context.Context) (release func(), err error) {
	for {
		d.mu.RLock()
		sem := d.sem
		done := d.genDone
		d.mu.RUnlock()
		if sem == nil {
			d.inflight.Add(1)
			return func() { d.inflight.Add(-1) }, nil
		}
		select {
		case sem <- struct{}{}:
			d.inflight.Add(1)
			return func() {
				<-sem
				d.inflight.Add(-1)
			}, nil
		case <-done:
			// Generation retired mid-wait — loop and re-read d.sem so we
			// respect the new limit rather than silently draining from the
			// detached old channel.
			continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
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
