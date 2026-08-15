package engine

import (
	"fmt"
	"strconv"

	"github.com/shopspring/decimal"
)

// Value is the runtime scalar used by graphs that declare a non-decimal
// VariableConfig.DataType.  Its tagged representation keeps strings and
// booleans distinct from numeric values until an operation explicitly permits
// conversion.  Decimal remains the legacy runtime for all-decimal graphs.
type Value struct {
	Kind    ValueKind
	Decimal Decimal
	String  string
	Bool    bool
}

type ValueKind string

const (
	ValueDecimal ValueKind = "decimal"
	ValueInteger ValueKind = "integer"
	ValueString  ValueKind = "string"
	ValueBoolean ValueKind = "boolean"
)

func (v Value) WireString() string {
	switch v.Kind {
	case ValueString:
		return v.String
	case ValueBoolean:
		return strconv.FormatBool(v.Bool)
	default:
		return v.Decimal.String()
	}
}

func (v Value) numeric() (Decimal, bool) {
	return v.Decimal, v.Kind == ValueDecimal || v.Kind == ValueInteger
}

func typedInput(raw, dataType string) (Value, error) {
	switch dataType {
	case "", "decimal":
		d, err := decimal.NewFromString(raw)
		if err != nil {
			return Value{}, fmt.Errorf("cannot parse %q as decimal: %w", raw, err)
		}
		return Value{Kind: ValueDecimal, Decimal: d}, nil
	case "integer":
		d, err := decimal.NewFromString(raw)
		if err != nil {
			return Value{}, fmt.Errorf("cannot parse %q as integer: %w", raw, err)
		}
		if !d.Equal(d.Truncate(0)) {
			return Value{}, fmt.Errorf("%q is not an integer", raw)
		}
		return Value{Kind: ValueInteger, Decimal: d}, nil
	case "string":
		return Value{Kind: ValueString, String: raw}, nil
	case "boolean":
		if raw == "true" {
			return Value{Kind: ValueBoolean, Bool: true}, nil
		}
		if raw == "false" {
			return Value{Kind: ValueBoolean, Bool: false}, nil
		}
		return Value{}, fmt.Errorf("cannot parse %q as boolean (expected true or false)", raw)
	default:
		return Value{}, fmt.Errorf("unsupported variable dataType %q", dataType)
	}
}
