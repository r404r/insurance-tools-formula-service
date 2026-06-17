package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

const (
	MaxRequestBodyBytes        int64 = 10 << 20 // 10 MiB
	MaxParseTextBytes                = 64 << 10 // 64 KiB
	MaxBatchCalculateInputSets       = 1000
	MaxBatchTestCases                = 5000
	MaxConcurrentCalcsLimit          = 10000
)

// writeJSON serializes v as JSON and writes it to w with the given HTTP status
// code. Sets Content-Type to application/json.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, ErrorResponse{
				Error: "request body too large",
				Code:  http.StatusRequestEntityTooLarge,
			})
			return false
		}
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Code:  http.StatusBadRequest,
		})
		return false
	}
	return true
}
