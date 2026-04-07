package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// Known setting keys.
const (
	SettingMaxConcurrentCalcs = "max_concurrent_calcs"
)

// SettingsHandler implements admin settings endpoints.
type SettingsHandler struct {
	Settings store.SettingsRepository
	Limiter  *DynamicConcurrencyLimiter
}

// Get returns all current settings.
// GET /api/v1/settings
func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	all, err := h.Settings.All(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to load settings", Code: http.StatusInternalServerError})
		return
	}

	resp := settingsToResponse(all, h.Limiter.Limit())
	writeJSON(w, http.StatusOK, resp)
}

// Update applies new settings values.
// PUT /api/v1/settings
func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req UpdateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}

	if req.MaxConcurrentCalcs != nil {
		v := *req.MaxConcurrentCalcs
		if v < 0 {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "maxConcurrentCalcs must be >= 0 (0 = unlimited)", Code: http.StatusBadRequest})
			return
		}
		if err := h.Settings.Set(r.Context(), SettingMaxConcurrentCalcs, strconv.Itoa(v)); err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("save setting: %s", err), Code: http.StatusInternalServerError})
			return
		}
		h.Limiter.SetLimit(v)
	}

	// Return updated state.
	all, err := h.Settings.All(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to reload settings", Code: http.StatusInternalServerError})
		return
	}
	writeJSON(w, http.StatusOK, settingsToResponse(all, h.Limiter.Limit()))
}

// settingsToResponse converts a raw key-value map to the typed response DTO.
// The live limiter value is authoritative (covers the case where the DB row
// has never been written yet).
func settingsToResponse(all map[string]string, liveLimit int) SettingsResponse {
	maxCalcs := liveLimit
	if v, ok := all[SettingMaxConcurrentCalcs]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			maxCalcs = n
		}
	}
	return SettingsResponse{MaxConcurrentCalcs: maxCalcs}
}
