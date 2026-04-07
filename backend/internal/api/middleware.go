package api

import (
	"net/http"
	"runtime/debug"
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

// ConcurrencyLimiter returns middleware that caps concurrent in-flight requests
// at n. When the cap is reached callers receive 503 Service Unavailable with a
// Retry-After: 1 header. Passing n ≤ 0 disables the limit (pass-through).
func ConcurrencyLimiter(n int) func(http.Handler) http.Handler {
	if n <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	sem := make(chan struct{}, n)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
