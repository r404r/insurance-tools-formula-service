package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/engine"
	"github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// stubEngine satisfies the CalculationEngine interface with a deterministic
// pure function and no shared mutable state, so we can stress BatchTest's
// parallel path under the race detector without pulling in the whole engine
// package's graph/cache machinery.
type stubEngine struct{}

func (stubEngine) Calculate(ctx context.Context, _ *domain.FormulaGraph, inputs map[string]string) (*engine.CalculationResult, error) {
	// Echo the input numeric `x` as the result so we can assert expected==actual.
	// Always allocate fresh maps — no sharing.
	x := inputs["x"]
	out := map[string]string{"result": x}
	intermediates := map[string]string{"x": x}
	return &engine.CalculationResult{
		Outputs:       out,
		Intermediates: intermediates,
		ExecutionTime: 1 * time.Microsecond,
	}, nil
}

func (stubEngine) Validate(_ *domain.FormulaGraph) []engine.ValidationError { return nil }
func (stubEngine) ClearCache()                                              {}
func (stubEngine) CacheStats() (int, int)                                   { return 0, 0 }

// stubVersionRepo returns a fixed FormulaVersion regardless of the ID/version
// requested, avoiding the need for a real store implementation. Only the
// GetPublished / GetVersion paths are exercised by BatchTest via
// h.resolveVersion — the other methods are stubs to satisfy the interface.
type stubVersionRepo struct{}

func (stubVersionRepo) CreateVersion(_ context.Context, _ *domain.FormulaVersion) error {
	return nil
}
func (stubVersionRepo) GetVersion(_ context.Context, _ string, _ int) (*domain.FormulaVersion, error) {
	return &domain.FormulaVersion{Version: 1}, nil
}
func (stubVersionRepo) GetPublished(_ context.Context, _ string) (*domain.FormulaVersion, error) {
	return &domain.FormulaVersion{Version: 1}, nil
}
func (stubVersionRepo) ListVersions(_ context.Context, _ string) ([]*domain.FormulaVersion, error) {
	return nil, nil
}
func (stubVersionRepo) UpdateState(_ context.Context, _ string, _ int, _ domain.VersionState) error {
	return nil
}

// Ensure stubVersionRepo satisfies the store interface at compile time.
var _ store.VersionRepository = stubVersionRepo{}

// TestBatchTestConcurrentRequestsRace stresses BatchTest with many concurrent
// HTTP calls, each containing many cases, under the race detector. It must
// not produce any "DATA RACE" reports or panics.
func TestBatchTestConcurrentRequestsRace(t *testing.T) {
	h := &CalcHandler{
		Engine:   stubEngine{},
		Versions: stubVersionRepo{},
		Limiter:  NewDynamicConcurrencyLimiter(16),
	}

	// Build a request body with 200 cases.
	cases := make([]BatchTestCase, 200)
	for i := 0; i < 200; i++ {
		xs := strconv.Itoa(i)
		cases[i] = BatchTestCase{
			Label:    "case-" + xs,
			Inputs:   map[string]string{"x": xs},
			Expected: map[string]string{"result": xs},
		}
	}
	body, err := json.Marshal(BatchTestRequest{
		FormulaID: "test-formula",
		Cases:     cases,
		Tolerance: "0.001", // non-zero so compareValues exercises the big.Float path
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Fire N concurrent requests against the handler.
	const concurrentRequests = 10
	var wg sync.WaitGroup
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/calculate/batch-test", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			h.BatchTest(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("unexpected status %d: %s", rr.Code, rr.Body.String())
				return
			}
			var resp BatchTestResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Errorf("unmarshal: %v", err)
				return
			}
			if resp.Summary.Total != 200 || resp.Summary.Passed != 200 {
				t.Errorf("summary total=%d passed=%d want 200/200", resp.Summary.Total, resp.Summary.Passed)
			}
			// Verify result order is preserved.
			for i, r := range resp.Results {
				want := "case-" + strconv.Itoa(i)
				if r.Label != want {
					t.Errorf("result %d label=%q want %q", i, r.Label, want)
					return
				}
			}
			if resp.Summary.TotalExecutionTimeMs <= 0 {
				t.Errorf("totalExecutionTimeMs should be > 0, got %v", resp.Summary.TotalExecutionTimeMs)
			}
		}()
	}
	wg.Wait()
}

// TestBatchTestSetLimitMidFlightRace mutates the limiter cap while batch
// requests are in progress, validating the SetLimit wake-up path and the
// generation-captured release closure under the race detector.
func TestBatchTestSetLimitMidFlightRace(t *testing.T) {
	h := &CalcHandler{
		Engine:   stubEngine{},
		Versions: stubVersionRepo{},
		Limiter:  NewDynamicConcurrencyLimiter(4),
	}

	cases := make([]BatchTestCase, 50)
	for i := 0; i < 50; i++ {
		xs := strconv.Itoa(i)
		cases[i] = BatchTestCase{
			Label:    "c" + xs,
			Inputs:   map[string]string{"x": xs},
			Expected: map[string]string{"result": xs},
		}
	}
	body, _ := json.Marshal(BatchTestRequest{FormulaID: "f", Cases: cases})

	// Flip the limit a few times while 5 batch requests run concurrently.
	var wg sync.WaitGroup
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/calculate/batch-test", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			h.BatchTest(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("status %d: %s", rr.Code, rr.Body.String())
			}
		}()
	}

	// Mutator goroutine: oscillate the global limit.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			h.Limiter.SetLimit(2)
			time.Sleep(1 * time.Millisecond)
			h.Limiter.SetLimit(10)
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()
	<-done
}
