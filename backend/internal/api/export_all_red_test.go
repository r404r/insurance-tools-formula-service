package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFormulaExportAcceptsMoreThanOneListPage(t *testing.T) {
	ids := make([]string, 501)
	for i := range ids {
		ids[i] = fmt.Sprintf("formula-%d", i)
	}
	body, _ := json.Marshal(ExportRequest{IDs: ids})
	rr := httptest.NewRecorder()
	(&FormulaHandler{Formulas: &alreadyDeletedFormulaRepo{}, Versions: newInMemoryVersionRepo()}).Export(rr, httptest.NewRequest(http.MethodPost, "/api/v1/formulas/export", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 501 IDs gathered across pages; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-Export-Requested"); got != "501" {
		t.Fatalf("X-Export-Requested = %q, want 501", got)
	}
}
