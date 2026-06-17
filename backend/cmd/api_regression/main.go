// Command api_regression is a black-box HTTP regression suite for the formula
// service API. It drives a *running* backend (default http://localhost:8080,
// PostgreSQL-backed in the containerized stack) through the core API surface,
// asserts the documented contracts, and writes a Markdown summary to disk so
// every run leaves a reviewable artifact.
//
// It is self-provisioning: it creates its own formula/version/test data via the
// API and cleans up afterwards, so it does not depend on any particular seed
// state beyond the default admin account and the bootstrap categories that the
// server always creates on a fresh database.
//
// Usage:
//
//	BASE_URL=http://localhost:8080 \
//	ADMIN_USER=admin ADMIN_PASS=admin99999 \
//	REPORT_DIR=tests/reports \
//	go run ./cmd/api_regression
//
// Exit code is 0 only when every check passes; any failure (or a skipped check
// caused by an upstream failure) exits 1 so CI / scripts can gate on it.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────
// Config
// ─────────────────────────────────────────────────────────────────────────

type config struct {
	BaseURL   string
	AdminUser string
	AdminPass string
	ReportDir string
}

func loadConfig() config {
	return config{
		BaseURL:   envOr("BASE_URL", "http://localhost:8080"),
		AdminUser: envOr("ADMIN_USER", "admin"),
		AdminPass: envOr("ADMIN_PASS", "admin99999"),
		ReportDir: envOr("REPORT_DIR", "tests/reports"),
	}
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// ─────────────────────────────────────────────────────────────────────────
// HTTP client
// ─────────────────────────────────────────────────────────────────────────

// client is a thin wrapper around http.Client. The cookie jar transparently
// carries the httpOnly auth_token cookie the server sets at login, so
// authenticated calls need no manual header wiring.
type client struct {
	base string
	http *http.Client
}

func newClient(base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{
		base: strings.TrimRight(base, "/"),
		http: &http.Client{Timeout: 30 * time.Second, Jar: jar},
	}
}

// noAuthClient returns a client that shares no cookies — used to assert that
// protected endpoints reject unauthenticated requests.
func noAuthClient(base string) *client { return newClient(base) }

type apiResponse struct {
	Status int
	Body   []byte
}

func (c *client) do(method, path string, body any) (apiResponse, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return apiResponse{}, fmt.Errorf("marshal body: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return apiResponse{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return apiResponse{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return apiResponse{}, err
	}
	return apiResponse{Status: resp.StatusCode, Body: data}, nil
}

func (r apiResponse) decode(v any) error { return json.Unmarshal(r.Body, v) }

// snippet returns a short, single-line excerpt of the body for report details.
func (r apiResponse) snippet() string {
	s := strings.TrimSpace(string(r.Body))
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// ─────────────────────────────────────────────────────────────────────────
// Result tracking
// ─────────────────────────────────────────────────────────────────────────

type status string

const (
	statusPass status = "PASS"
	statusFail status = "FAIL"
	statusSkip status = "SKIP"
)

type result struct {
	Group    string
	Name     string
	Status   status
	Detail   string
	Duration time.Duration
}

type suite struct {
	results []result
	aborted bool // a foundational step failed; dependent checks should skip
}

// check runs fn, timing it and recording the outcome. fn returns a short
// success detail and/or an error. When the suite is already aborted, the check
// is recorded as skipped without running fn.
func (s *suite) check(group, name string, fn func() (string, error)) bool {
	if s.aborted {
		s.results = append(s.results, result{Group: group, Name: name, Status: statusSkip, Detail: "skipped — earlier critical step failed"})
		return false
	}
	start := time.Now()
	detail, err := fn()
	dur := time.Since(start)
	if err != nil {
		s.results = append(s.results, result{Group: group, Name: name, Status: statusFail, Detail: err.Error(), Duration: dur})
		return false
	}
	s.results = append(s.results, result{Group: group, Name: name, Status: statusPass, Detail: detail, Duration: dur})
	return true
}

// critical behaves like check but aborts the remaining suite on failure — used
// for foundational steps (login, formula/version setup) that everything else
// depends on.
func (s *suite) critical(group, name string, fn func() (string, error)) bool {
	ok := s.check(group, name, fn)
	if !ok && !s.aborted {
		s.aborted = true
	}
	return ok
}

func (s *suite) counts() (pass, fail, skip int) {
	for _, r := range s.results {
		switch r.Status {
		case statusPass:
			pass++
		case statusFail:
			fail++
		case statusSkip:
			skip++
		}
	}
	return
}

// ─────────────────────────────────────────────────────────────────────────
// Assertions
// ─────────────────────────────────────────────────────────────────────────

func wantStatus(r apiResponse, want int) error {
	if r.Status != want {
		return fmt.Errorf("expected HTTP %d, got %d: %s", want, r.Status, r.snippet())
	}
	return nil
}

// numericEqual compares two decimal strings within a small relative tolerance,
// tolerating the engine's trailing-zero padding (e.g. "57" vs "57.000…0").
func numericEqual(a, b string) bool {
	fa, ea := strconv.ParseFloat(strings.TrimSpace(a), 64)
	fb, eb := strconv.ParseFloat(strings.TrimSpace(b), 64)
	if ea != nil || eb != nil {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}
	diff := fa - fb
	if diff < 0 {
		diff = -diff
	}
	scale := 1.0
	if fb < 0 {
		scale = -fb
	} else if fb > 0 {
		scale = fb
	}
	return diff <= 1e-9*scale || diff <= 1e-9
}

// ─────────────────────────────────────────────────────────────────────────
// Main
// ─────────────────────────────────────────────────────────────────────────

func main() {
	cfg := loadConfig()
	startedAt := time.Now().UTC()

	c := newClient(cfg.BaseURL)
	s := &suite{}

	// Shared state threaded through dependent checks.
	var (
		dbDriver  string
		graphRaw  json.RawMessage
		outputID  string
		formulaID string
		tableID   string
	)

	// 1) Readiness / health ------------------------------------------------
	s.critical("health", "GET /healthz returns ok", func() (string, error) {
		// Retry briefly: the backend may still be migrating when the script
		// points us at a freshly started container.
		var last error
		for i := 0; i < 30; i++ {
			r, err := c.do(http.MethodGet, "/healthz", nil)
			if err == nil && r.Status == http.StatusOK {
				var h struct {
					Status   string `json:"status"`
					Database string `json:"database"`
				}
				if err := r.decode(&h); err != nil {
					return "", fmt.Errorf("decode health: %w", err)
				}
				dbDriver = h.Database
				return fmt.Sprintf("status=%s database=%s", h.Status, h.Database), nil
			}
			last = err
			time.Sleep(time.Second)
		}
		if last != nil {
			return "", fmt.Errorf("backend not ready: %w", last)
		}
		return "", fmt.Errorf("backend not ready at %s", cfg.BaseURL)
	})

	// 2) Auth --------------------------------------------------------------
	s.critical("auth", "POST /auth/login (admin) succeeds", func() (string, error) {
		r, err := c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
			"username": cfg.AdminUser, "password": cfg.AdminPass,
		})
		if err != nil {
			return "", err
		}
		if err := wantStatus(r, http.StatusOK); err != nil {
			return "", err
		}
		var lr struct {
			Token string `json:"token"`
			User  struct {
				Username string `json:"username"`
				Role     string `json:"role"`
			} `json:"user"`
		}
		if err := r.decode(&lr); err != nil {
			return "", err
		}
		if lr.Token == "" {
			return "", fmt.Errorf("login returned empty token")
		}
		return fmt.Sprintf("role=%s", lr.User.Role), nil
	})

	s.check("auth", "POST /auth/login wrong password rejected", func() (string, error) {
		r, err := c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
			"username": cfg.AdminUser, "password": "definitely-wrong-pass",
		})
		if err != nil {
			return "", err
		}
		return "401 as expected", wantStatus(r, http.StatusUnauthorized)
	})

	s.check("auth", "GET /auth/me returns current admin", func() (string, error) {
		r, err := c.do(http.MethodGet, "/api/v1/auth/me", nil)
		if err != nil {
			return "", err
		}
		if err := wantStatus(r, http.StatusOK); err != nil {
			return "", err
		}
		var u struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		}
		if err := r.decode(&u); err != nil {
			return "", err
		}
		if u.Username != cfg.AdminUser {
			return "", fmt.Errorf("expected username %q, got %q", cfg.AdminUser, u.Username)
		}
		return fmt.Sprintf("username=%s role=%s", u.Username, u.Role), nil
	})

	s.check("auth", "GET /auth/me without auth rejected", func() (string, error) {
		r, err := noAuthClient(cfg.BaseURL).do(http.MethodGet, "/api/v1/auth/me", nil)
		if err != nil {
			return "", err
		}
		return "401 as expected", wantStatus(r, http.StatusUnauthorized)
	})

	// 3) Public endpoints --------------------------------------------------
	s.check("public", "GET /templates returns catalogue", func() (string, error) {
		r, err := c.do(http.MethodGet, "/api/v1/templates", nil)
		if err != nil {
			return "", err
		}
		if err := wantStatus(r, http.StatusOK); err != nil {
			return "", err
		}
		var env struct {
			Templates []json.RawMessage `json:"templates"`
		}
		if err := r.decode(&env); err != nil {
			return "", fmt.Errorf("templates envelope decode: %w", err)
		}
		if len(env.Templates) == 0 {
			return "", fmt.Errorf("templates catalogue is empty")
		}
		return fmt.Sprintf("%d templates", len(env.Templates)), nil
	})

	s.critical("public", "POST /parse converts text to DAG", func() (string, error) {
		r, err := c.do(http.MethodPost, "/api/v1/parse", map[string]string{
			"text": "principal * rate + base",
		})
		if err != nil {
			return "", err
		}
		if err := wantStatus(r, http.StatusOK); err != nil {
			return "", err
		}
		var pr struct {
			Graph json.RawMessage `json:"graph"`
		}
		if err := r.decode(&pr); err != nil {
			return "", err
		}
		graphRaw = pr.Graph
		var g struct {
			Nodes   []json.RawMessage `json:"nodes"`
			Outputs []string          `json:"outputs"`
		}
		if err := json.Unmarshal(pr.Graph, &g); err != nil {
			return "", err
		}
		if len(g.Nodes) == 0 || len(g.Outputs) == 0 {
			return "", fmt.Errorf("parsed graph has no nodes/outputs")
		}
		outputID = g.Outputs[0]
		return fmt.Sprintf("%d nodes, output=%s", len(g.Nodes), outputID), nil
	})

	// 4) Categories --------------------------------------------------------
	s.check("categories", "GET /categories lists bootstrap categories", func() (string, error) {
		r, err := c.do(http.MethodGet, "/api/v1/categories", nil)
		if err != nil {
			return "", err
		}
		if err := wantStatus(r, http.StatusOK); err != nil {
			return "", err
		}
		var env struct {
			Categories []struct {
				Slug string `json:"slug"`
			} `json:"categories"`
		}
		if err := r.decode(&env); err != nil {
			return "", err
		}
		if len(env.Categories) < 1 {
			return "", fmt.Errorf("expected at least one category, got 0")
		}
		return fmt.Sprintf("%d categories", len(env.Categories)), nil
	})

	// 5) Authorization boundary -------------------------------------------
	s.check("authz", "POST /formulas without auth rejected", func() (string, error) {
		r, err := noAuthClient(cfg.BaseURL).do(http.MethodPost, "/api/v1/formulas", map[string]string{
			"name": "unauthorized", "domain": "life",
		})
		if err != nil {
			return "", err
		}
		return "401 as expected", wantStatus(r, http.StatusUnauthorized)
	})

	// 6) Formula lifecycle (self-provisioned) -----------------------------
	formulaName := "regression-" + startedAt.Format("20060102T150405Z")

	s.critical("formula", "POST /formulas creates formula", func() (string, error) {
		r, err := c.do(http.MethodPost, "/api/v1/formulas", map[string]string{
			"name":        formulaName,
			"domain":      "life",
			"description": "API regression self-provisioned formula",
		})
		if err != nil {
			return "", err
		}
		if err := wantStatus(r, http.StatusCreated); err != nil {
			return "", err
		}
		var f struct {
			ID string `json:"id"`
		}
		if err := r.decode(&f); err != nil {
			return "", err
		}
		if f.ID == "" {
			return "", fmt.Errorf("created formula has empty id")
		}
		formulaID = f.ID
		return "id=" + f.ID, nil
	})

	s.check("formula", "GET /formulas/{id} returns formula", func() (string, error) {
		r, err := c.do(http.MethodGet, "/api/v1/formulas/"+formulaID, nil)
		if err != nil {
			return "", err
		}
		return "200", wantStatus(r, http.StatusOK)
	})

	s.critical("formula", "POST /formulas/{id}/versions creates draft v1", func() (string, error) {
		r, err := c.do(http.MethodPost, "/api/v1/formulas/"+formulaID+"/versions", map[string]any{
			"graph":      graphRaw,
			"changeNote": "regression initial version",
		})
		if err != nil {
			return "", err
		}
		if err := wantStatus(r, http.StatusCreated); err != nil {
			return "", err
		}
		var v struct {
			Version int    `json:"version"`
			State   string `json:"state"`
		}
		if err := r.decode(&v); err != nil {
			return "", err
		}
		if v.Version != 1 || v.State != "draft" {
			return "", fmt.Errorf("expected version 1 draft, got version %d state %q", v.Version, v.State)
		}
		return "version=1 state=draft", nil
	})

	s.critical("formula", "PATCH version 1 -> published", func() (string, error) {
		r, err := c.do(http.MethodPatch, "/api/v1/formulas/"+formulaID+"/versions/1", map[string]string{
			"state": "published",
		})
		if err != nil {
			return "", err
		}
		return "200", wantStatus(r, http.StatusOK)
	})

	// 7) Calculation -------------------------------------------------------
	// principal*rate + base = 1000*0.05 + 7 = 57
	s.check("calculate", "POST /calculate returns expected result", func() (string, error) {
		r, err := c.do(http.MethodPost, "/api/v1/calculate", map[string]any{
			"formulaId": formulaID,
			"inputs":    map[string]string{"principal": "1000", "rate": "0.05", "base": "7"},
		})
		if err != nil {
			return "", err
		}
		if err := wantStatus(r, http.StatusOK); err != nil {
			return "", err
		}
		var resp struct {
			Result map[string]string `json:"result"`
		}
		if err := r.decode(&resp); err != nil {
			return "", err
		}
		got, ok := resp.Result[outputID]
		if !ok {
			return "", fmt.Errorf("result missing output key %q: %v", outputID, resp.Result)
		}
		if !numericEqual(got, "57") {
			return "", fmt.Errorf("expected 57, got %s", got)
		}
		return "result=" + got, nil
	})

	s.check("calculate", "POST /calculate/validate accepts graph", func() (string, error) {
		r, err := c.do(http.MethodPost, "/api/v1/calculate/validate", graphRaw)
		if err != nil {
			return "", err
		}
		if err := wantStatus(r, http.StatusOK); err != nil {
			return "", err
		}
		var v struct {
			Valid bool `json:"valid"`
		}
		if err := r.decode(&v); err != nil {
			return "", err
		}
		if !v.Valid {
			return "", fmt.Errorf("parsed graph reported invalid: %s", r.snippet())
		}
		return "valid=true", nil
	})

	s.check("calculate", "POST /calculate/batch-test passes all cases", func() (string, error) {
		cases := []map[string]any{
			{"label": "case-1", "inputs": map[string]string{"principal": "1000", "rate": "0.05", "base": "7"}, "expected": map[string]string{outputID: "57"}},
			{"label": "case-2", "inputs": map[string]string{"principal": "2000", "rate": "0.10", "base": "0"}, "expected": map[string]string{outputID: "200"}},
			{"label": "case-3", "inputs": map[string]string{"principal": "0", "rate": "0.10", "base": "42"}, "expected": map[string]string{outputID: "42"}},
		}
		r, err := c.do(http.MethodPost, "/api/v1/calculate/batch-test", map[string]any{
			"formulaId": formulaID,
			"tolerance": "0.0001",
			"cases":     cases,
		})
		if err != nil {
			return "", err
		}
		if err := wantStatus(r, http.StatusOK); err != nil {
			return "", err
		}
		var resp struct {
			Summary struct {
				Total    int     `json:"total"`
				Passed   int     `json:"passed"`
				Failed   int     `json:"failed"`
				PassRate float64 `json:"passRate"`
			} `json:"summary"`
		}
		if err := r.decode(&resp); err != nil {
			return "", err
		}
		if resp.Summary.Failed != 0 || resp.Summary.Passed != resp.Summary.Total {
			return "", fmt.Errorf("batch-test had failures: %d/%d passed", resp.Summary.Passed, resp.Summary.Total)
		}
		return fmt.Sprintf("%d/%d passed (%.0f%%)", resp.Summary.Passed, resp.Summary.Total, resp.Summary.PassRate), nil
	})

	s.check("formula", "GET /formulas includes new formula", func() (string, error) {
		r, err := c.do(http.MethodGet, "/api/v1/formulas", nil)
		if err != nil {
			return "", err
		}
		if err := wantStatus(r, http.StatusOK); err != nil {
			return "", err
		}
		var env struct {
			Formulas []struct {
				ID string `json:"id"`
			} `json:"formulas"`
			Total int `json:"total"`
		}
		if err := r.decode(&env); err != nil {
			return "", err
		}
		for _, f := range env.Formulas {
			if f.ID == formulaID {
				return fmt.Sprintf("found in list (total=%d)", env.Total), nil
			}
		}
		return "", fmt.Errorf("created formula %s not present in list of %d", formulaID, env.Total)
	})

	// 8) Lookup tables -----------------------------------------------------
	tableName := "regression-table-" + startedAt.Format("20060102T150405Z")
	s.check("tables", "GET /tables returns list", func() (string, error) {
		r, err := c.do(http.MethodGet, "/api/v1/tables", nil)
		if err != nil {
			return "", err
		}
		return "200", wantStatus(r, http.StatusOK)
	})

	tableCreated := s.check("tables", "POST /tables creates lookup table", func() (string, error) {
		r, err := c.do(http.MethodPost, "/api/v1/tables", map[string]any{
			"name":      tableName,
			"domain":    "life",
			"tableType": "single_key",
			"data":      []map[string]string{{"key": "1", "value": "10"}, {"key": "2", "value": "20"}},
		})
		if err != nil {
			return "", err
		}
		if err := wantStatus(r, http.StatusCreated); err != nil {
			return "", err
		}
		var t struct {
			ID string `json:"id"`
		}
		if err := r.decode(&t); err != nil {
			return "", err
		}
		tableID = t.ID
		return "id=" + t.ID, nil
	})

	if tableCreated && tableID != "" {
		s.check("tables", "DELETE /tables/{id} cleans up", func() (string, error) {
			r, err := c.do(http.MethodDelete, "/api/v1/tables/"+tableID, nil)
			if err != nil {
				return "", err
			}
			return "204", wantStatus(r, http.StatusNoContent)
		})
	}

	// 9) Cleanup -----------------------------------------------------------
	if formulaID != "" {
		s.check("cleanup", "DELETE /formulas/{id} removes test formula", func() (string, error) {
			r, err := c.do(http.MethodDelete, "/api/v1/formulas/"+formulaID, nil)
			if err != nil {
				return "", err
			}
			return "204", wantStatus(r, http.StatusNoContent)
		})
	}

	// Report ---------------------------------------------------------------
	finishedAt := time.Now().UTC()
	pass, fail, skip := s.counts()
	if dbDriver == "" {
		dbDriver = "unknown"
	}

	report := renderReport(cfg, dbDriver, startedAt, finishedAt, s.results, pass, fail, skip)
	paths, werr := writeReport(cfg.ReportDir, startedAt, report)
	if werr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write report: %v\n", werr)
	}

	// Console summary.
	fmt.Printf("\nAPI regression against %s (db=%s)\n", cfg.BaseURL, dbDriver)
	fmt.Printf("  PASS=%d FAIL=%d SKIP=%d  (%s)\n", pass, fail, skip, finishedAt.Sub(startedAt).Round(time.Millisecond))
	for _, p := range paths {
		fmt.Printf("  report: %s\n", p)
	}

	if fail > 0 || skip > 0 {
		os.Exit(1)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Reporting
// ─────────────────────────────────────────────────────────────────────────

func renderReport(cfg config, dbDriver string, start, finish time.Time, results []result, pass, fail, skip int) string {
	var b strings.Builder
	overall := "✅ PASS"
	if fail > 0 || skip > 0 {
		overall = "❌ FAIL"
	}
	total := pass + fail + skip

	fmt.Fprintf(&b, "# API Regression Report\n\n")
	fmt.Fprintf(&b, "- **Result:** %s — %d/%d checks passed\n", overall, pass, total)
	fmt.Fprintf(&b, "- **Run at (UTC):** %s\n", start.Format(time.RFC3339))
	fmt.Fprintf(&b, "- **Target:** %s\n", cfg.BaseURL)
	fmt.Fprintf(&b, "- **Database:** %s\n", dbDriver)
	fmt.Fprintf(&b, "- **Duration:** %s\n", finish.Sub(start).Round(time.Millisecond))
	fmt.Fprintf(&b, "- **Totals:** PASS=%d FAIL=%d SKIP=%d\n\n", pass, fail, skip)

	fmt.Fprintf(&b, "## Checks\n\n")
	fmt.Fprintf(&b, "| # | Group | Check | Result | Duration | Detail |\n")
	fmt.Fprintf(&b, "|---|-------|-------|--------|---------:|--------|\n")
	for i, r := range results {
		icon := "✅"
		switch r.Status {
		case statusFail:
			icon = "❌"
		case statusSkip:
			icon = "⏭️"
		}
		dur := "—"
		if r.Duration > 0 {
			dur = r.Duration.Round(time.Millisecond).String()
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s %s | %s | %s |\n",
			i+1, r.Group, r.Name, icon, r.Status, dur, mdEscape(r.Detail))
	}

	if fail > 0 || skip > 0 {
		fmt.Fprintf(&b, "\n## Failures & Skips\n\n")
		for _, r := range results {
			if r.Status == statusPass {
				continue
			}
			fmt.Fprintf(&b, "- **[%s] %s — %s**: %s\n", r.Status, r.Group, r.Name, mdEscape(r.Detail))
		}
	}

	fmt.Fprintf(&b, "\n---\n_Generated by `backend/cmd/api_regression`. Re-run with `tests/api-regression/run.sh`._\n")
	return b.String()
}

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// writeReport writes a timestamped report plus an overwritten "-latest" copy so
// the most recent run is always at a stable path while history is preserved.
func writeReport(dir string, start time.Time, content string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	stamped := filepath.Join(dir, "api-regression-"+start.Format("20060102T150405Z")+".md")
	latest := filepath.Join(dir, "api-regression-latest.md")
	if err := os.WriteFile(stamped, []byte(content), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(latest, []byte(content), 0o644); err != nil {
		return []string{stamped}, err
	}
	return []string{stamped, latest}, nil
}
