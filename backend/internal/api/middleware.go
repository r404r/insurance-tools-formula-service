package api

import (
	"net/http"
	"runtime/debug"
	"sync"
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
// Swapping the limit creates a fresh buffered channel; in-flight requests
// draining the old channel are harmless — the channel is garbage-collected
// once all slot-holders release it.
type DynamicConcurrencyLimiter struct {
	mu    sync.RWMutex
	sem   chan struct{}
	limit int
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

// SetLimit replaces the semaphore with a new one of capacity n.
// n ≤ 0 disables the limit.
func (d *DynamicConcurrencyLimiter) SetLimit(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.limit = n
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

// Middleware returns an http.Handler middleware that enforces the current limit.
// When the cap is reached the handler returns 503 + Retry-After: 1 immediately.
func (d *DynamicConcurrencyLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			d.mu.RLock()
			sem := d.sem
			d.mu.RUnlock()

			if sem == nil {
				next.ServeHTTP(w, r)
				return
			}

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
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
