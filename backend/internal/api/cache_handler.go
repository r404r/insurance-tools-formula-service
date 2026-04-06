package api

import (
	"net/http"
)

// CacheHandler implements cache management HTTP endpoints.
type CacheHandler struct {
	Engine CalculationEngine
}

// Stats returns current cache statistics.
// GET /api/v1/cache
func (h *CacheHandler) Stats(w http.ResponseWriter, r *http.Request) {
	size, maxSize := h.Engine.CacheStats()
	writeJSON(w, http.StatusOK, map[string]any{
		"size":    size,
		"maxSize": maxSize,
	})
}

// Clear removes all entries from the result cache.
// DELETE /api/v1/cache
func (h *CacheHandler) Clear(w http.ResponseWriter, r *http.Request) {
	h.Engine.ClearCache()
	w.WriteHeader(http.StatusNoContent)
}
