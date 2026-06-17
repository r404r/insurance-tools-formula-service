package api

import (
	"net/http"
	"strings"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/parser"
)

// ParseHandler exposes text-to-graph conversion via HTTP.
type ParseHandler struct{}

// Parse converts a text formula expression into a FormulaGraph (DAG).
// POST /api/v1/parse
func (h *ParseHandler) Parse(w http.ResponseWriter, r *http.Request) {
	var req ParseRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "text is required", Code: http.StatusBadRequest})
		return
	}
	if len(text) > MaxParseTextBytes {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "text is too long", Code: http.StatusBadRequest})
		return
	}

	p := parser.NewParser(text)
	ast, err := p.Parse()
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{Error: err.Error(), Code: http.StatusUnprocessableEntity})
		return
	}

	graph, err := parser.ASTToDAG(ast)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{Error: err.Error(), Code: http.StatusUnprocessableEntity})
		return
	}

	writeJSON(w, http.StatusOK, ParseResponse{Graph: *graph})
}
