package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONReturns413ForOversizedBody(t *testing.T) {
	handler := MaxRequestBody(8)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var dst map[string]string
		if !decodeJSON(w, r, &dst) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"value":"too large"}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestBatchCalculateRejectsTooManyInputSets(t *testing.T) {
	inputSets := make([]map[string]string, MaxBatchCalculateInputSets+1)
	for i := range inputSets {
		inputSets[i] = map[string]string{"x": "1"}
	}
	body := marshalTestJSON(t, BatchCalculateRequest{
		FormulaID: "formula-1",
		InputSets: inputSets,
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/calculate/batch", strings.NewReader(body))
	(&CalcHandler{}).BatchCalculate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestBatchTestRejectsTooManyCases(t *testing.T) {
	cases := make([]BatchTestCase, MaxBatchTestCases+1)
	for i := range cases {
		cases[i] = BatchTestCase{
			Inputs:   map[string]string{"x": "1"},
			Expected: map[string]string{"y": "1"},
		}
	}
	body := marshalTestJSON(t, BatchTestRequest{
		FormulaID: "formula-1",
		Cases:     cases,
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/calculate/batch-test", strings.NewReader(body))
	(&CalcHandler{}).BatchTest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestParseRejectsTooLongText(t *testing.T) {
	body := marshalTestJSON(t, ParseRequest{
		Text: strings.Repeat("x", MaxParseTextBytes+1),
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/parse", strings.NewReader(body))
	(&ParseHandler{}).Parse(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestConcurrencyLimitIsNormalized(t *testing.T) {
	limiter := NewDynamicConcurrencyLimiter(MaxConcurrentCalcsLimit + 1)
	if got := limiter.Limit(); got != MaxConcurrentCalcsLimit {
		t.Fatalf("initial limit = %d, want %d", got, MaxConcurrentCalcsLimit)
	}

	limiter.SetLimit(MaxConcurrentCalcsLimit + 100)
	if got := limiter.Limit(); got != MaxConcurrentCalcsLimit {
		t.Fatalf("updated limit = %d, want %d", got, MaxConcurrentCalcsLimit)
	}

	resp := settingsToResponse(map[string]string{SettingMaxConcurrentCalcs: "2000000"}, limiter.Limit())
	if resp.MaxConcurrentCalcs != MaxConcurrentCalcsLimit {
		t.Fatalf("settings response limit = %d, want %d", resp.MaxConcurrentCalcs, MaxConcurrentCalcsLimit)
	}
}
