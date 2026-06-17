package api

import (
	"context"
	"net/http"
	"time"
)

// HealthHandler exposes a lightweight, unauthenticated readiness probe used by
// container orchestration (docker healthcheck) and the API regression suite to
// wait for the backend to become ready before issuing requests.
type HealthHandler struct {
	// Ping verifies the database connection is alive. Injected from the
	// store so the handler does not import a concrete store package.
	Ping func(ctx context.Context) error
	// Driver is the active database driver name (sqlite/postgres/mysql),
	// echoed back so an operator can confirm which backend is serving.
	Driver string
}

// HealthResponse is the JSON body returned by GET /healthz.
type HealthResponse struct {
	Status   string `json:"status"`          // "ok" or "degraded"
	Database string `json:"database"`        // active driver name
	Error    string `json:"error,omitempty"` // populated when status != ok
}

// Health responds 200 when the database is reachable, 503 otherwise.
// GET /healthz
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if h.Ping != nil {
		if err := h.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, HealthResponse{
				Status:   "degraded",
				Database: h.Driver,
				Error:    err.Error(),
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, HealthResponse{
		Status:   "ok",
		Database: h.Driver,
	})
}
