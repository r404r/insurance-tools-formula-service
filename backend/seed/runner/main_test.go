package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubstitute_ReplacesFormulaAndTablePlaceholders(t *testing.T) {
	formulas := map[string]string{
		"生存率因子 1-qx": "f-uuid-1",
		"死亡給付PV項":    "f-uuid-2",
	}
	tables := map[string]string{
		"日本標準生命表2007（簡易版）": "t-uuid-1",
		"claims_triangle_sample": "t-uuid-2",
	}
	in := []byte(`{
  "config": {
    "formulaId": "{{formula:生存率因子 1-qx}}",
    "tableId": "{{table:日本標準生命表2007（簡易版）}}",
    "iterator": "k"
  },
  "agg": {
    "tableId": "{{table:claims_triangle_sample}}",
    "ref": "{{formula:死亡給付PV項}}"
  }
}`)
	out, err := substitute(in, formulas, tables)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		`"formulaId": "f-uuid-1"`,
		`"tableId": "t-uuid-1"`,
		`"tableId": "t-uuid-2"`,
		`"ref": "f-uuid-2"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, got)
		}
	}
	if strings.Contains(got, "{{") {
		t.Errorf("output still contains placeholder tokens:\n%s", got)
	}
}

func TestSubstitute_ReportsUnresolved(t *testing.T) {
	in := []byte(`{"a":"{{formula:missing}}","b":"{{table:nope}}","c":"{{formula:also missing}}"}`)
	_, err := substitute(in, map[string]string{}, map[string]string{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"formula:missing", "table:nope", "formula:also missing"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q: %s", want, msg)
		}
	}
}

func TestSubstitute_NoOpWhenNoPlaceholders(t *testing.T) {
	in := []byte(`{"plain":"json","without":"tokens"}`)
	out, err := substitute(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(in) {
		t.Errorf("expected unchanged output, got %s", out)
	}
}

func TestSubstitute_DoesNotMatchOpenBraceWithoutPrefix(t *testing.T) {
	// Some descriptions / comments may legitimately contain `{{` without
	// the formula:/table: prefix; the regex must leave them alone.
	in := []byte(`{"description":"sum {{from a to b}} placeholder unrelated"}`)
	out, err := substitute(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(in) {
		t.Errorf("expected unchanged output, got %s", out)
	}
}

func TestPeekFormulaName(t *testing.T) {
	bundle := []byte(`{
  "version":"1.0",
  "formulas":[{"name":"hello","domain":"life","graph":{"nodes":[],"edges":[],"outputs":[]}}]
}`)
	name, err := peekFormulaName(bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "hello" {
		t.Errorf("got %q, want hello", name)
	}
}

func TestPeekFormulaName_RejectsMultiFormulaBundle(t *testing.T) {
	bundle := []byte(`{"formulas":[{"name":"a"},{"name":"b"}]}`)
	if _, err := peekFormulaName(bundle); err == nil {
		t.Fatal("expected error for multi-formula bundle")
	}
}

func TestPeekFormulaName_RejectsEmptyBundle(t *testing.T) {
	bundle := []byte(`{"formulas":[]}`)
	if _, err := peekFormulaName(bundle); err == nil {
		t.Fatal("expected error for empty bundle")
	}
}

// writeTestSeedDir lays out a tiny in-tree seed/ tree (one table, two
// formulas where the second references the first via a placeholder) so we
// can drive the runner end-to-end without a real backend.
func writeTestSeedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "tables"))
	mustMkdir(t, filepath.Join(dir, "formulas"))

	mustWriteJSON(t, filepath.Join(dir, "tables", "010-rates.json"), map[string]any{
		"name":      "rates",
		"domain":    "life",
		"tableType": "rate",
		"data":      []any{},
	})
	mustWriteJSON(t, filepath.Join(dir, "formulas", "010-body.json"), map[string]any{
		"version": "1.0",
		"formulas": []any{
			map[string]any{
				"name":   "body",
				"domain": "life",
				"graph": map[string]any{
					"nodes":   []any{},
					"edges":   []any{},
					"outputs": []any{},
				},
			},
		},
	})
	mustWriteJSON(t, filepath.Join(dir, "formulas", "020-consumer.json"), map[string]any{
		"version": "1.0",
		"formulas": []any{
			map[string]any{
				"name":   "consumer",
				"domain": "life",
				"graph": map[string]any{
					"nodes": []any{
						map[string]any{
							"id":   "loop",
							"type": "loop",
							"config": map[string]any{
								"formulaId": "{{formula:body}}",
								"tableId":   "{{table:rates}}",
							},
						},
					},
					"edges":   []any{},
					"outputs": []any{},
				},
			},
		},
	})
	return dir
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRun_DryRunResolvesAllPlaceholders(t *testing.T) {
	dir := writeTestSeedDir(t)
	cfg := config{SeedDir: dir, DryRun: true}
	if err := run(cfg); err != nil {
		t.Fatalf("dry-run should succeed against valid bundles: %v", err)
	}
}

func TestRun_DryRunFailsWhenPlaceholderUnresolved(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "tables"))
	mustMkdir(t, filepath.Join(dir, "formulas"))
	mustWriteJSON(t, filepath.Join(dir, "formulas", "010-broken.json"), map[string]any{
		"version": "1.0",
		"formulas": []any{
			map[string]any{
				"name":   "broken",
				"domain": "life",
				"graph": map[string]any{
					"nodes": []any{
						map[string]any{
							"id":   "n",
							"type": "loop",
							"config": map[string]any{
								"formulaId": "{{formula:does-not-exist}}",
							},
						},
					},
					"edges":   []any{},
					"outputs": []any{},
				},
			},
		},
	})
	cfg := config{SeedDir: dir, DryRun: true}
	err := run(cfg)
	if err == nil {
		t.Fatal("expected dry-run to fail on unresolved placeholder")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error should name the missing dependency, got: %v", err)
	}
}

func TestRun_OnlyFiltersDependencies_DryRun(t *testing.T) {
	dir := writeTestSeedDir(t)
	cfg := config{SeedDir: dir, DryRun: true, Only: "consumer"}
	// Even though we only ask for "consumer", the runner must still resolve
	// the {{formula:body}} and {{table:rates}} placeholders against
	// synthetic dry-run ids minted for the filtered-out files.
	if err := run(cfg); err != nil {
		t.Fatalf("dry-run --only should succeed when deps are minted as DRY-RUN ids: %v", err)
	}
}

func TestRun_OnlyTypoFailsLoudly(t *testing.T) {
	dir := writeTestSeedDir(t)
	cfg := config{SeedDir: dir, DryRun: true, Only: "this-name-does-not-exist"}
	err := run(cfg)
	if err == nil {
		t.Fatal("expected --only with no matches to return an error")
	}
	if !strings.Contains(err.Error(), "did not match") {
		t.Errorf("error message should mention 'did not match', got: %v", err)
	}
}

func TestFindDryRunRefs(t *testing.T) {
	resolved := []byte(`{
  "config": {
    "formulaId": "DRY-RUN-FORMULA-生存率因子 1-qx",
    "tableId": "DRY-RUN-TABLE-rates",
    "other": "real-uuid-not-flagged"
  },
  "dup": "DRY-RUN-FORMULA-生存率因子 1-qx"
}`)
	got := findDryRunRefs(resolved)
	want := []string{
		"formula:生存率因子 1-qx",
		"table:rates",
	}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d] want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestFindDryRunRefs_NoMatches(t *testing.T) {
	resolved := []byte(`{"config":{"formulaId":"abc-123","tableId":"def-456"}}`)
	if got := findDryRunRefs(resolved); len(got) != 0 {
		t.Errorf("expected no matches, got %v", got)
	}
}

