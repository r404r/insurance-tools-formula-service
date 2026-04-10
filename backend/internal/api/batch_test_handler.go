package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

// batchWorkerDefaultMax is the fallback worker cap when the global
// concurrency limit is unlimited (0). It also clamps the computed
// cap so an absurdly large global limit cannot spawn unbounded
// goroutines per batch request.
const batchWorkerDefaultMax = 8

// computeBatchWorkers returns the number of parallel workers used by
// BatchTest, given the current global concurrency limit.
//
// Rule: Batch workers = floor(globalLimit / 5), clamped to
// [1, batchWorkerDefaultMax]. When globalLimit <= 0 (unlimited),
// fall back to batchWorkerDefaultMax.
//
// The 1/5 ratio reserves at least 4/5 of the global calculation
// budget for concurrent non-batch requests, so a large batch run
// cannot starve interactive calculations.
func computeBatchWorkers(globalLimit int) int {
	if globalLimit <= 0 {
		return batchWorkerDefaultMax
	}
	w := globalLimit / 5
	if w < 1 {
		w = 1
	}
	if w > batchWorkerDefaultMax {
		w = batchWorkerDefaultMax
	}
	return w
}

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

	// Parallel execution via a fixed worker pool. Two properties we need:
	//
	//   1. Total concurrent calculations (across all single + batch requests)
	//      stays within the globally configured cap. Each inner case acquires
	//      a slot from the shared limiter via Acquire/Release; the batch-test
	//      route is NOT gated by the HTTP middleware (see router.go) so the
	//      outer request does not double-count.
	//
	//   2. Per-request goroutine count is bounded by `workers`, not by
	//      len(Cases). A fixed worker pool pulling jobs from a buffered
	//      channel means a 10k-case upload still only creates ~W goroutines.
	//
	// Results are indexed into a pre-allocated slice so output order matches
	// input order regardless of which worker processes which case.
	results := make([]BatchTestCaseResult, len(req.Cases))

	globalLimit := 0
	if h.Limiter != nil {
		globalLimit = h.Limiter.Limit()
	}
	workers := computeBatchWorkers(globalLimit)
	// Never launch more workers than there are cases.
	if workers > len(req.Cases) {
		workers = len(req.Cases)
	}

	type caseJob struct {
		i  int
		tc BatchTestCase
	}
	jobs := make(chan caseJob, workers)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	batchStart := time.Now()

	// Producer: feed cases into the jobs channel. Stops early if ctx is done
	// (client disconnect) so workers can drain and exit.
	go func() {
		defer close(jobs)
		for i, tc := range req.Cases {
			select {
			case jobs <- caseJob{i, tc}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Workers: fixed pool, each acquires a shared-limiter slot per case.
	// Each case runs in its own inline closure so:
	//   1. A panic in runBatchTestCase is recovered locally, recorded as an
	//      error on the case result, and the limiter slot is released.
	//      Without this, a bad formula/input could crash the server AND leak
	//      a semaphore slot, since the http recovery middleware sits on the
	//      handler goroutine, not on workers.
	//   2. defer release() uses the closure returned from Acquire, which
	//      captures the specific semaphore generation that was written to.
	//      If SetLimit swaps the semaphore mid-batch, the drain still lands
	//      on the old channel — the new semaphore's capacity stays accurate.
	var wg sync.WaitGroup
	for wIdx := 0; wIdx < workers; wIdx++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				runOneBatchCase(ctx, h.Limiter, h.Engine, &version.Graph, job.i, job.tc, tolerance, results)
			}
		}()
	}
	wg.Wait()

	// If the client disconnected mid-run, surface that clearly rather than
	// returning a partially-populated result set as if it were complete.
	if ctx.Err() != nil {
		writeJSON(w, http.StatusRequestTimeout, ErrorResponse{
			Error: "batch test cancelled: " + ctx.Err().Error(),
			Code:  http.StatusRequestTimeout,
		})
		return
	}
	totalElapsed := time.Since(batchStart)

	total := len(req.Cases)
	passed := 0
	for _, r := range results {
		if r.Pass {
			passed++
		}
	}
	failed := total - passed
	passRate := 0.0
	if total > 0 {
		passRate = float64(passed) / float64(total) * 100
	}

	writeJSON(w, http.StatusOK, BatchTestResponse{
		Summary: BatchTestSummary{
			Total:                total,
			Passed:               passed,
			Failed:               failed,
			PassRate:             passRate,
			TotalExecutionTimeMs: float64(totalElapsed.Microseconds()) / 1000.0,
		},
		Results: results,
	})
}

// runOneBatchCase is the per-case worker body. It owns:
//   - acquiring a shared-limiter slot (blocks on ctx)
//   - recovering panics from the engine or comparison code
//   - writing the result into the caller-provided slice at the correct index
//
// Isolated into its own function so `defer` unwinds in a predictable order
// (recover → release → slot write) per case, without leaking across workers.
func runOneBatchCase(
	ctx context.Context,
	limiter *DynamicConcurrencyLimiter,
	engine CalculationEngine,
	graph *domain.FormulaGraph,
	index int,
	tc BatchTestCase,
	tolerance *big.Float,
	results []BatchTestCaseResult,
) {
	// Local error recorder used for both acquire-failure and panic paths.
	recordErr := func(msg string) {
		results[index] = BatchTestCaseResult{
			Index:    index + 1,
			Label:    tc.Label,
			Inputs:   tc.Inputs,
			Expected: tc.Expected,
			Actual:   map[string]string{},
			Error:    msg,
		}
	}

	var release func()
	if limiter != nil {
		r, err := limiter.Acquire(ctx)
		if err != nil {
			recordErr("cancelled: " + err.Error())
			return
		}
		release = r
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	defer func() {
		if rv := recover(); rv != nil {
			recordErr(fmt.Sprintf("panic: %v", rv))
		}
	}()

	results[index] = runBatchTestCase(ctx, engine, graph, index, tc, tolerance)
}

// runBatchTestCase executes a single case and returns the populated result.
// Factored out of BatchTest so the parallel loop body stays small.
func runBatchTestCase(
	ctx context.Context,
	engine CalculationEngine,
	graph *domain.FormulaGraph,
	index int,
	tc BatchTestCase,
	tolerance *big.Float,
) BatchTestCaseResult {
	caseResult := BatchTestCaseResult{
		Index:    index + 1,
		Label:    tc.Label,
		Inputs:   tc.Inputs,
		Expected: tc.Expected,
	}

	res, calcErr := engine.Calculate(ctx, graph, tc.Inputs)
	if calcErr != nil {
		caseResult.Pass = false
		caseResult.Error = calcErr.Error()
		caseResult.Actual = map[string]string{}
		return caseResult
	}

	caseResult.Actual = res.Outputs
	caseResult.ExecutionTimeMs = float64(res.ExecutionTime.Microseconds()) / 1000.0
	caseResult.CacheHit = res.CacheHit

	// A case with no expected keys cannot be verified.
	if len(tc.Expected) == 0 {
		caseResult.Pass = false
		caseResult.Error = "no expected values specified"
		return caseResult
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
	return caseResult
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
