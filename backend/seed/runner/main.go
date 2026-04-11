// Command seed-runner loads built-in seed data into a running formula-service
// instance via its public HTTP API. It replaces the legacy hard-coded seed
// inside backend/cmd/server/main.go.
//
// Layout (relative to --seed-dir, default ./backend/seed):
//
//	tables/    *.json    one CreateTableRequest per file (sorted by name)
//	formulas/  *.json    one ExportBundle per file (single formula),
//	                     sorted by filename. Cross-references between
//	                     formulas / tables use placeholder tokens:
//	                       {{table:NAME}}
//	                       {{formula:NAME}}
//	                     The runner substitutes them at import time using
//	                     the names of objects already created in this run
//	                     plus any objects that already existed in the DB.
//
// Idempotency: tables and formulas are looked up by name before creation;
// existing objects are skipped. Re-running the seed runner against a DB
// that already has all seed data is a no-op (zero writes).
//
// Caveat: this is a list-then-create check, not a transactional upsert.
// Backend storage has no name uniqueness constraint, so a concurrent
// create from another client between the runner's GET and POST will lead
// to a duplicate row. Treat the runner as a deployment-time bootstrap; do
// not run it concurrently with normal user traffic.
//
// Authentication: the runner logs in as the bootstrap admin (default
// admin/admin99999, overridable via flags or env). The bootstrap admin is
// still created by backend/cmd/server/main.go because the API has no
// chicken-and-egg admin endpoint.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ---- CLI ----

type config struct {
	BaseURL   string
	AdminUser string
	AdminPass string
	SeedDir   string
}

func main() {
	cfg := config{
		BaseURL:   envOr("SEED_BASE_URL", "http://localhost:8080"),
		AdminUser: envOr("SEED_ADMIN_USER", "admin"),
		AdminPass: envOr("SEED_ADMIN_PASS", "admin99999"),
		SeedDir:   envOr("SEED_DIR", "backend/seed"),
	}
	flag.StringVar(&cfg.BaseURL, "base-url", cfg.BaseURL, "formula-service base URL")
	flag.StringVar(&cfg.AdminUser, "admin-user", cfg.AdminUser, "bootstrap admin username")
	flag.StringVar(&cfg.AdminPass, "admin-pass", cfg.AdminPass, "bootstrap admin password")
	flag.StringVar(&cfg.SeedDir, "seed-dir", cfg.SeedDir, "directory containing tables/ and formulas/ subdirs")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "seed runner failed: %v\n", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	c := newClient(cfg.BaseURL)

	fmt.Printf("[seed-runner] base=%s seed-dir=%s\n", cfg.BaseURL, cfg.SeedDir)

	// 1. Login (with retry). The backend has no /healthz endpoint, so we
	// use the login itself as the readiness probe — it proves the HTTP
	// server is up, the DB is reachable, and the bootstrap admin user
	// exists. This matters for docker compose: seed-runner can start
	// before backend finishes initializing.
	if err := c.loginWithRetry(cfg.AdminUser, cfg.AdminPass, 60*time.Second); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	fmt.Println("[seed-runner] login ok")

	// 3. Pre-load existing object name → id maps from the running DB so we
	// can resolve placeholders that point at objects created in earlier
	// runs (or by other seed sources).
	tables, err := c.listTables()
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}
	formulas, err := c.listFormulas()
	if err != nil {
		return fmt.Errorf("list formulas: %w", err)
	}

	// 4. Seed tables (sorted filename order).
	tableFiles, err := listJSON(filepath.Join(cfg.SeedDir, "tables"))
	if err != nil {
		return fmt.Errorf("list table files: %w", err)
	}
	tableCreated, tableSkipped := 0, 0
	for _, path := range tableFiles {
		var req createTableRequest
		if err := readJSON(path, &req); err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if req.Name == "" {
			return fmt.Errorf("%s: name is required", path)
		}
		if id, ok := tables[req.Name]; ok {
			fmt.Printf("[seed-runner]  table skip  %s (exists, id=%s)\n", req.Name, short(id))
			tableSkipped++
			continue
		}
		id, err := c.createTable(req)
		if err != nil {
			return fmt.Errorf("create table %s: %w", req.Name, err)
		}
		tables[req.Name] = id
		fmt.Printf("[seed-runner]  table OK    %s (id=%s)\n", req.Name, short(id))
		tableCreated++
	}

	// 5. Seed formulas (sorted filename order; dependency order encoded in
	// filename prefix). Each file is a single-formula ExportBundle whose
	// graph may contain {{formula:NAME}} / {{table:NAME}} placeholders.
	formulaFiles, err := listJSON(filepath.Join(cfg.SeedDir, "formulas"))
	if err != nil {
		return fmt.Errorf("list formula files: %w", err)
	}
	formulaCreated, formulaSkipped := 0, 0
	for _, path := range formulaFiles {
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		// Peek the name without consuming the bundle so we can check for
		// idempotency before doing any substitution work.
		name, err := peekFormulaName(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if id, ok := formulas[name]; ok {
			fmt.Printf("[seed-runner]  formula skip %s (exists, id=%s)\n", name, short(id))
			formulaSkipped++
			continue
		}
		// Substitute placeholders against the maps accumulated so far.
		resolved, err := substitute(raw, formulas, tables)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		id, err := c.importFormula(resolved)
		if err != nil {
			return fmt.Errorf("import %s: %w", path, err)
		}
		// Imported formulas are created as Draft. Publish version 1 so the
		// list page and downstream consumers see the same state as the
		// legacy hard-coded seed produced.
		if err := c.publishVersion(id, 1); err != nil {
			return fmt.Errorf("publish %s: %w", name, err)
		}
		formulas[name] = id
		fmt.Printf("[seed-runner]  formula OK   %s (id=%s)\n", name, short(id))
		formulaCreated++
	}

	fmt.Printf("[seed-runner] done — tables: %d created, %d skipped — formulas: %d created, %d skipped\n",
		tableCreated, tableSkipped, formulaCreated, formulaSkipped)
	return nil
}

// ---- HTTP client ----

type client struct {
	baseURL string
	http    *http.Client
	token   string
}

func newClient(baseURL string) *client {
	return &client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// loginWithRetry repeatedly attempts to log in until success or timeout.
// Connection-refused / DNS errors / 5xx responses are retried; auth-shape
// errors (4xx like 401 invalid credentials) fail fast since retrying with
// the same credentials will never succeed.
func (c *client) loginWithRetry(user, pass string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	attempt := 0
	for {
		attempt++
		err := c.login(user, pass)
		if err == nil {
			return nil
		}
		// Fail fast on 4xx — retrying with the same credentials is futile.
		var fatal *httpStatusError
		if errors.As(err, &fatal) && fatal.status >= 400 && fatal.status < 500 {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("after %d attempts: %w", attempt, err)
		}
		if attempt == 1 {
			fmt.Printf("[seed-runner] backend not ready, retrying: %v\n", err)
		}
		time.Sleep(time.Second)
	}
}

func (c *client) login(user, pass string) error {
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	req, _ := http.NewRequest(http.MethodPost, c.baseURL+"/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return httpError(resp)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if out.Token == "" {
		return errors.New("login response missing token")
	}
	c.token = out.Token
	return nil
}

func (c *client) doJSON(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	req, _ := http.NewRequest(method, c.baseURL+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return httpError(resp)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *client) doRaw(method, path string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, c.baseURL+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return httpError(resp)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *client) listTables() (map[string]string, error) {
	var resp struct {
		Tables []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"tables"`
	}
	if err := c.doJSON(http.MethodGet, "/api/v1/tables", nil, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(resp.Tables))
	for _, r := range resp.Tables {
		out[r.Name] = r.ID
	}
	return out, nil
}

func (c *client) listFormulas() (map[string]string, error) {
	out := make(map[string]string)
	// Paginate using the API's limit/offset query params (the formula list
	// handler ignores anything else, see formula_handler.go). The seed set
	// is small but other code paths can add more, so don't assume one page
	// covers everything.
	const pageSize = 200
	offset := 0
	for {
		q := url.Values{}
		q.Set("limit", fmt.Sprintf("%d", pageSize))
		q.Set("offset", fmt.Sprintf("%d", offset))
		var resp struct {
			Formulas []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"formulas"`
			Total int `json:"total"`
		}
		if err := c.doJSON(http.MethodGet, "/api/v1/formulas?"+q.Encode(), nil, &resp); err != nil {
			return nil, err
		}
		for _, f := range resp.Formulas {
			out[f.Name] = f.ID
		}
		offset += len(resp.Formulas)
		if len(resp.Formulas) < pageSize || offset >= resp.Total {
			break
		}
		if offset > 100000 {
			return nil, errors.New("listFormulas runaway pagination")
		}
	}
	return out, nil
}

func (c *client) createTable(req createTableRequest) (string, error) {
	var resp struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(http.MethodPost, "/api/v1/tables", req, &resp); err != nil {
		return "", err
	}
	if resp.ID == "" {
		return "", errors.New("create table returned empty id")
	}
	return resp.ID, nil
}

func (c *client) importFormula(bundle []byte) (string, error) {
	var resp struct {
		Imported []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Index int    `json:"index"`
		} `json:"imported"`
		Errors []struct {
			Index int    `json:"index"`
			Name  string `json:"name"`
			Error string `json:"error"`
		} `json:"errors"`
	}
	if err := c.doRaw(http.MethodPost, "/api/v1/formulas/import", bundle, &resp); err != nil {
		return "", err
	}
	if len(resp.Errors) > 0 {
		return "", fmt.Errorf("import error: %s", resp.Errors[0].Error)
	}
	if len(resp.Imported) == 0 {
		return "", errors.New("import returned no imported items")
	}
	return resp.Imported[0].ID, nil
}

func (c *client) publishVersion(formulaID string, version int) error {
	path := fmt.Sprintf("/api/v1/formulas/%s/versions/%d", formulaID, version)
	body := map[string]string{"state": "published"}
	return c.doJSON(http.MethodPatch, path, body, nil)
}

// ---- File / format helpers ----

type createTableRequest struct {
	Name      string          `json:"name"`
	Domain    string          `json:"domain"`
	TableType string          `json:"tableType"`
	Data      json.RawMessage `json:"data"`
}

func listJSON(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

func readJSON(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// peekFormulaName extracts the first formula's name from a single-entry
// ExportBundle without doing any placeholder substitution.
func peekFormulaName(raw []byte) (string, error) {
	var bundle struct {
		Formulas []struct {
			Name string `json:"name"`
		} `json:"formulas"`
	}
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return "", err
	}
	if len(bundle.Formulas) != 1 {
		return "", fmt.Errorf("expected exactly 1 formula in bundle, found %d", len(bundle.Formulas))
	}
	return bundle.Formulas[0].Name, nil
}

// placeholderRe matches {{formula:NAME}} and {{table:NAME}} tokens. The name
// part allows any character that isn't a closing brace, including whitespace
// and CJK letters used by the seed formulas.
var placeholderRe = regexp.MustCompile(`\{\{(formula|table):([^}]+)\}\}`)

// substitute replaces placeholders in raw bundle bytes with real IDs from
// the supplied maps. Unresolved placeholders return an error so the runner
// fails fast instead of POSTing a malformed bundle to the backend.
func substitute(raw []byte, formulas, tables map[string]string) ([]byte, error) {
	var unresolved []string
	out := placeholderRe.ReplaceAllFunc(raw, func(match []byte) []byte {
		m := placeholderRe.FindSubmatch(match)
		kind := string(m[1])
		name := string(m[2])
		var lookup map[string]string
		if kind == "formula" {
			lookup = formulas
		} else {
			lookup = tables
		}
		id, ok := lookup[name]
		if !ok {
			unresolved = append(unresolved, fmt.Sprintf("%s:%s", kind, name))
			return match
		}
		return []byte(id)
	})
	if len(unresolved) > 0 {
		return nil, fmt.Errorf("unresolved placeholders: %s", strings.Join(unresolved, ", "))
	}
	return out, nil
}

// ---- Misc helpers ----

// httpStatusError carries the HTTP status code so callers can use errors.As
// to distinguish retryable (5xx) vs fatal (4xx) failures without parsing
// strings.
type httpStatusError struct {
	status int
	path   string
	body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("http %d %s: %s", e.status, e.path, e.body)
}

func httpError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return &httpStatusError{
		status: resp.StatusCode,
		path:   resp.Request.URL.Path,
		body:   strings.TrimSpace(string(body)),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
