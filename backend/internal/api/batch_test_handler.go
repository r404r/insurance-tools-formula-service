package api

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
)

// BatchTest executes a set of test cases against a formula and compares actual
// outputs to expected values.
// POST /api/v1/calculate/batch-test
func (h *CalcHandler) BatchTest(w http.ResponseWriter, r *http.Request) {
	var req BatchTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body", Code: http.StatusBadRequest})
		return
	}
	if req.FormulaID == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "formulaId is required", Code: http.StatusBadRequest})
		return
	}
	if len(req.Cases) == 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "at least one test case is required", Code: http.StatusBadRequest})
		return
	}

	// Parse tolerance (relative). Default 0 → exact match.
	tolerance := new(big.Float).SetPrec(64)
	if req.Tolerance != "" {
		if _, ok := tolerance.SetString(req.Tolerance); !ok {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid tolerance value", Code: http.StatusBadRequest})
			return
		}
		if tolerance.Sign() < 0 {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "tolerance must be >= 0", Code: http.StatusBadRequest})
			return
		}
	}

	version, err := h.resolveVersion(r.Context(), req.FormulaID, req.Version)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "formula version not found", Code: http.StatusNotFound})
		return
	}

	results := make([]BatchTestCaseResult, 0, len(req.Cases))
	passed := 0

	for i, tc := range req.Cases {
		res, calcErr := h.Engine.Calculate(r.Context(), &version.Graph, tc.Inputs)

		caseResult := BatchTestCaseResult{
			Index:    i + 1,
			Label:    tc.Label,
			Inputs:   tc.Inputs,
			Expected: tc.Expected,
		}

		if calcErr != nil {
			caseResult.Pass = false
			caseResult.Error = calcErr.Error()
			caseResult.Actual = map[string]string{}
			results = append(results, caseResult)
			continue
		}

		caseResult.Actual = res.Outputs
		caseResult.ExecutionTimeMs = float64(res.ExecutionTime.Microseconds()) / 1000.0
		caseResult.CacheHit = res.CacheHit

		// A case with no expected keys cannot be verified.
		if len(tc.Expected) == 0 {
			caseResult.Pass = false
			caseResult.Error = "no expected values specified"
			results = append(results, caseResult)
			continue
		}

		// Compare each expected output key.
		allPass := true
		diff := map[string]string{}
		for key, expStr := range tc.Expected {
			actStr, ok := res.Outputs[key]
			if !ok {
				allPass = false
				diff[key] = fmt.Sprintf("missing output key %q", key)
				continue
			}
			match, d, cmpErr := compareValues(expStr, actStr, tolerance)
			if cmpErr != nil {
				allPass = false
				diff[key] = cmpErr.Error()
				continue
			}
			if !match {
				allPass = false
				diff[key] = d
			}
		}

		caseResult.Pass = allPass
		if len(diff) > 0 {
			caseResult.Diff = diff
		}
		if allPass {
			passed++
		}
		results = append(results, caseResult)
	}

	total := len(req.Cases)
	failed := total - passed
	passRate := 0.0
	if total > 0 {
		passRate = float64(passed) / float64(total) * 100
	}

	writeJSON(w, http.StatusOK, BatchTestResponse{
		Summary: BatchTestSummary{
			Total:    total,
			Passed:   passed,
			Failed:   failed,
			PassRate: passRate,
		},
		Results: results,
	})
}

// compareValues returns whether actual matches expected within the given
// relative tolerance, plus a human-readable diff string when they don't match.
func compareValues(expected, actual string, tolerance *big.Float) (bool, string, error) {
	exp := new(big.Float).SetPrec(128)
	if _, ok := exp.SetString(expected); !ok {
		return false, "", fmt.Errorf("cannot parse expected value %q as number", expected)
	}
	act := new(big.Float).SetPrec(128)
	if _, ok := act.SetString(actual); !ok {
		return false, "", fmt.Errorf("cannot parse actual value %q as number", actual)
	}

	// diff = |actual - expected|
	absDiff := new(big.Float).SetPrec(128).Sub(act, exp)
	if absDiff.Sign() < 0 {
		absDiff.Neg(absDiff)
	}

	isZeroTol := tolerance.Sign() == 0
	isZeroExp := exp.Sign() == 0

	if isZeroTol {
		// Exact match: compare string representations after normalisation.
		if exp.Cmp(act) == 0 {
			return true, "", nil
		}
		return false, fmt.Sprintf("expected %s, got %s", expected, actual), nil
	}

	// Relative tolerance: |actual - expected| <= tolerance * |expected|
	var threshold *big.Float
	if isZeroExp {
		// When expected is 0 use tolerance as absolute threshold.
		threshold = new(big.Float).SetPrec(128).Set(tolerance)
	} else {
		absExp := new(big.Float).SetPrec(128).Set(exp)
		if absExp.Sign() < 0 {
			absExp.Neg(absExp)
		}
		threshold = new(big.Float).SetPrec(128).Mul(tolerance, absExp)
	}

	if absDiff.Cmp(threshold) <= 0 {
		return true, "", nil
	}

	diffStr, _ := absDiff.Float64()
	return false, fmt.Sprintf("expected %s, got %s (diff %.6g)", expected, actual, diffStr), nil
}
