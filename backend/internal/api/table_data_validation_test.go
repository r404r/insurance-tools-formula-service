package api

import (
	"encoding/json"
	"testing"
)

func TestValidateTableDataRequiresJSONArray(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
	}{
		{name: "absent", data: nil},
		{name: "null", data: json.RawMessage(`null`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateTableData(tt.data); err == nil {
				t.Fatal("validateTableData returned nil, want an error for missing/non-array data")
			}
		})
	}
}

func TestValidateTableDataAllowsEmptyJSONArray(t *testing.T) {
	if err := validateTableData(json.RawMessage(`[]`)); err != nil {
		t.Fatalf("validateTableData([]) = %v, want nil", err)
	}
}
