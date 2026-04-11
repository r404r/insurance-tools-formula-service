// Package seed exposes the canonical seed bundle file set so the backend
// binary can know which formula and table names are "seed-owned" without
// reading from the filesystem at runtime. This is needed by the
// /admin/reset-seed handler in cmd/server/main.go, which runs inside a
// docker container that does not have access to the developer's
// backend/seed/ directory.
//
// The actual loading-and-importing work is performed by the
// out-of-process seed-runner CLI (see cmd directory backend/seed/runner),
// which reads the same JSON files from disk so seed authors can edit them
// without rebuilding the backend image. Embedding here is a one-way
// snapshot of the bundle name list at backend build time.
package seed

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed formulas/*.json tables/*.json
var bundleFS embed.FS

// Names returns the set of formula names and table names declared by the
// embedded seed bundles. Both maps are deterministic snapshots of what was
// on disk when the backend binary was built; updating bundles requires a
// rebuild for the reset handler to track new seeds.
func Names() (formulas map[string]bool, tables map[string]bool, err error) {
	formulas = map[string]bool{}
	tables = map[string]bool{}
	if err := walk("formulas", func(name string, _ []byte) error {
		formulas[name] = true
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("read formula names: %w", err)
	}
	if err := walk("tables", func(name string, _ []byte) error {
		tables[name] = true
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("read table names: %w", err)
	}
	return formulas, tables, nil
}

// walk iterates the JSON files in subdir and calls fn for each one. fn
// receives the parsed seed object's name (formulas[0].name for formula
// bundles, top-level "name" for table seeds) along with the raw bytes.
func walk(subdir string, fn func(name string, raw []byte) error) error {
	entries, err := fs.ReadDir(bundleFS, subdir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		path := subdir + "/" + e.Name()
		raw, err := fs.ReadFile(bundleFS, path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		var probe struct {
			Name     string `json:"name"`
			Formulas []struct {
				Name string `json:"name"`
			} `json:"formulas"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			return fmt.Errorf("%s: invalid JSON: %w", path, err)
		}
		var name string
		switch {
		case len(probe.Formulas) > 0:
			name = probe.Formulas[0].Name
		case probe.Name != "":
			name = probe.Name
		default:
			return fmt.Errorf("%s: cannot determine seed name", path)
		}
		if err := fn(name, raw); err != nil {
			return err
		}
	}
	return nil
}
