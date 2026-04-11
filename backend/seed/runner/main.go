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
	DryRun    bool
	Only      string
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
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "parse and validate bundles without contacting the backend")
	flag.StringVar(&cfg.Only, "only", "", "if set, only seed the bundle whose name matches this exact string (formula name or table name)")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "seed runner failed: %v\n", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	mode := "live"
	if cfg.DryRun {
		mode = "dry-run"
	}
	fmt.Printf("[seed-runner] base=%s seed-dir=%s mode=%s only=%q\n", cfg.BaseURL, cfg.SeedDir, mode, cfg.Only)

	tableFiles, err := listJSON(filepath.Join(cfg.SeedDir, "tables"))
	if err != nil {
		return fmt.Errorf("list table files: %w", err)
	}
	formulaFiles, err := listJSON(filepath.Join(cfg.SeedDir, "formulas"))
	if err != nil {
		return fmt.Errorf("list formula files: %w", err)
	}

	// In dry-run mode we don't talk to the backend at all. The "existing"
	// maps stay empty so every bundle looks new, but the substitute() pass
	// is fed the names from earlier files in the same run, so we can still
	// validate that all placeholders resolve to something declared
	// in-tree. Failing here means the on-disk bundles are inconsistent.
	var c *client
	tables := map[string]string{}
	formulas := map[string]string{}
	if !cfg.DryRun {
		c = newClient(cfg.BaseURL)
		// Login (with retry). The backend has no /healthz endpoint, so we
		// use the login itself as the readiness probe — it proves the HTTP
		// server is up, the DB is reachable, and the bootstrap admin user
		// exists. This matters for docker compose: seed-runner can start
		// before backend finishes initializing.
		if err := c.loginWithRetry(cfg.AdminUser, cfg.AdminPass, 60*time.Second); err != nil {
			return fmt.Errorf("login: %w", err)
		}
		fmt.Println("[seed-runner] login ok")

		// Pre-load existing object name → id maps from the running DB so we
		// can resolve placeholders that point at objects created in earlier
		// runs (or by other seed sources).
		tables, err = c.listTables()
		if err != nil {
			return fmt.Errorf("list tables: %w", err)
		}
		formulas, err = c.listFormulas()
		if err != nil {
			return fmt.Errorf("list formulas: %w", err)
		}
	}

	// onlyMatched tracks whether --only ever matched any bundle. If the
	// flag was set but matched nothing (e.g. typo, stale name), the run
	// exits non-zero so the operator notices instead of seeing a silent
	// success message with 0 created and 33 filtered.
	onlyMatched := false

	// Seed tables (sorted filename order). In dry-run mode we still walk
	// the entire list so the formula loop's placeholder resolution gets a
	// complete table-name → fake-id map; we mint synthetic ids of the form
	// "DRY-RUN-TABLE-<name>" so substitution doesn't fail and the resulting
	// JSON is still valid (the dry-run runner never sends it anywhere).
	tableCreated, tableSkipped, tableFiltered := 0, 0, 0
	for _, path := range tableFiles {
		var req createTableRequest
		if err := readJSON(path, &req); err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if req.Name == "" {
			return fmt.Errorf("%s: name is required", path)
		}
		if cfg.Only != "" && cfg.Only != req.Name {
			// Same as the formula loop: even when filtered out, register
			// a placeholder id so a downstream formula's
			// {{table:NAME}} placeholder still resolves. In live mode the
			// real id may already be in the prefetched `tables` map; only
			// mint a synthetic one if it isn't.
			if _, ok := tables[req.Name]; !ok {
				tables[req.Name] = "DRY-RUN-TABLE-" + req.Name
			}
			tableFiltered++
			continue
		}
		if cfg.Only != "" {
			// Once we've matched the table, refuse to also match a
			// formula with the same name on the next loop. --only is
			// documented as "single bundle".
			onlyMatched = true
		}
		if id, ok := tables[req.Name]; ok && !strings.HasPrefix(id, "DRY-RUN-") {
			fmt.Printf("[seed-runner]  table skip  %s (exists, id=%s)\n", req.Name, short(id))
			tableSkipped++
			continue
		}
		if cfg.DryRun {
			tables[req.Name] = "DRY-RUN-TABLE-" + req.Name
			fmt.Printf("[seed-runner]  table dry   %s (would create)\n", req.Name)
			tableCreated++
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

	// Seed formulas (sorted filename order; dependency order encoded in
	// filename prefix). Each file is a single-formula ExportBundle whose
	// graph may contain {{formula:NAME}} / {{table:NAME}} placeholders.
	//
	// When --only is set, we still walk every file in order so that
	// formulas earlier in dependency order get their names registered in
	// the resolution map (as DRY-RUN ids when not actually creating them);
	// otherwise an --only that targets a downstream consumer would fail
	// to substitute the upstream placeholder. We just skip the actual
	// import/publish for non-matching files.
	formulaCreated, formulaSkipped, formulaFiltered := 0, 0, 0
	for _, path := range formulaFiles {
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		name, err := peekFormulaName(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		filtered := cfg.Only != "" && cfg.Only != name
		if filtered {
			// Mint a dry-run id so downstream consumers can resolve.
			if _, ok := formulas[name]; !ok {
				formulas[name] = "DRY-RUN-FORMULA-" + name
			}
			formulaFiltered++
			continue
		}
		if cfg.Only != "" {
			if onlyMatched {
				// Same name as a table the table loop already created.
				// Surface the collision so a future seed author doesn't
				// silently double-create.
				return fmt.Errorf("--only %q matched both a table and a formula; rename one of them", cfg.Only)
			}
			onlyMatched = true
		}
		if id, ok := formulas[name]; ok && !strings.HasPrefix(id, "DRY-RUN-") {
			fmt.Printf("[seed-runner]  formula skip %s (exists, id=%s)\n", name, short(id))
			formulaSkipped++
			continue
		}
		// Substitute placeholders against the maps accumulated so far.
		// Failure here is one way dry-run produces a non-zero exit code,
		// which is exactly the validation we want.
		resolved, err := substitute(raw, formulas, tables)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		// In live mode, refuse to POST a bundle whose graph still
		// references a synthetic DRY-RUN-* id. This happens when --only
		// filtered out a dependency the current formula needs and that
		// dependency wasn't already in the prefetched DB. Surface the
		// specific missing dependency names so the operator knows what
		// to seed first.
		if !cfg.DryRun {
			if missing := findDryRunRefs(resolved); len(missing) > 0 {
				return fmt.Errorf("%s: refusing to import %q — its graph references %d dependency the current --only run did not satisfy and that the database did not already contain: %s. Seed those first or re-run without --only", path, name, len(missing), strings.Join(missing, ", "))
			}
		}
		if cfg.DryRun {
			formulas[name] = "DRY-RUN-FORMULA-" + name
			fmt.Printf("[seed-runner]  formula dry  %s (would create, %d bytes resolved)\n", name, len(resolved))
			formulaCreated++
			continue
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

	fmt.Printf("[seed-runner] done — tables: %d created, %d skipped, %d filtered — formulas: %d created, %d skipped, %d filtered\n",
		tableCreated, tableSkipped, tableFiltered, formulaCreated, formulaSkipped, formulaFiltered)

	// If --only was set but matched nothing, exit non-zero so the operator
	// notices a typo or stale name instead of seeing a quiet success line
	// with 0 actual work done. The onlyMatched flag is set whenever a
	// bundle file's name matched, regardless of whether the path went to
	// create or skip-existing.
	if cfg.Only != "" && !onlyMatched {
		return fmt.Errorf("--only %q did not match any bundle in %s/{tables,formulas}", cfg.Only, cfg.SeedDir)
	}

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

// dryRunRe matches the synthetic ids the runner mints when --only or
// --dry-run filters out a real backend create. We use it after
// substitution to detect (in live mode) that a placeholder was resolved
// to a placeholder, and to extract the dependency name for the error
// message. Names can contain spaces and CJK, so the only terminator we
// trust is the closing JSON string quote — synthetic ids only ever
// appear inside JSON string values produced by substitute().
var dryRunRe = regexp.MustCompile(`DRY-RUN-(FORMULA|TABLE)-([^"]+)`)

// findDryRunRefs returns a deduped, sorted list of synthetic dependency
// references found in resolved bundle bytes. Each entry has the form
// "kind:NAME" so the operator can copy the name verbatim into a follow-up
// --only invocation.
func findDryRunRefs(resolved []byte) []string {
	matches := dryRunRe.FindAllSubmatch(resolved, -1)
	seen := map[string]bool{}
	out := []string{}
	for _, m := range matches {
		kind := strings.ToLower(string(m[1]))
		name := string(m[2])
		key := kind + ":" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

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
