package api

import (
	"encoding/json"
	"net/http"
)

// writeJSON serializes v as JSON and writes it to w with the given HTTP status
// code. Sets Content-Type to application/json.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
