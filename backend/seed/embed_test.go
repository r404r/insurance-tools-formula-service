package seed

import "testing"

func TestNames_NonEmpty(t *testing.T) {
	formulas, tables, err := Names()
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if len(formulas) == 0 {
		t.Error("expected at least one formula name from embedded bundles")
	}
	if len(tables) == 0 {
		t.Error("expected at least one table name from embedded bundles")
	}
}

func TestNames_KnownSeedsPresent(t *testing.T) {
	// Spot-check a few canonical seed names that any future maintainer
	// would notice if accidentally dropped from the bundle set. These
	// match the names baked into docs/specs/002 and the LDF smoke test.
	formulas, tables, err := Names()
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	wantFormulas := []string{
		"寿险净保费计算",
		"日本損害保険 チェインラダー LDF",
		"生存率因子 1-qx",
	}
	for _, name := range wantFormulas {
		if !formulas[name] {
			t.Errorf("expected embedded formula name %q, not found", name)
		}
	}
	wantTables := []string{
		"日本標準生命表2007（簡易版）",
		"claims_triangle_sample",
	}
	for _, name := range wantTables {
		if !tables[name] {
			t.Errorf("expected embedded table name %q, not found", name)
		}
	}
}
