package main

import (
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
