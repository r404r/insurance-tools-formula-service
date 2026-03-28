package engine

import "github.com/shopspring/decimal"

// Decimal is a type alias for shopspring/decimal.Decimal, providing
// arbitrary-precision fixed-point arithmetic for insurance calculations.
type Decimal = decimal.Decimal

// RoundingMode controls how values are rounded during calculation.
type RoundingMode int

const (
	RoundHalfUp RoundingMode = iota
	RoundHalfDown
	RoundDown
	RoundUp
	RoundBankers
)

// PrecisionConfig controls decimal precision for intermediate and final results.
type PrecisionConfig struct {
	// IntermediatePrecision is the number of decimal places kept during
	// intermediate calculations to minimize accumulated rounding error.
	IntermediatePrecision int32

	// OutputPrecision is the number of decimal places in final output values.
	OutputPrecision int32

	// Rounding determines the rounding strategy applied to output values.
	Rounding RoundingMode
}

// DefaultPrecision returns a sensible default configuration for insurance
// premium calculations: 16 digits intermediate, 8 digits output, half-up rounding.
func DefaultPrecision() PrecisionConfig {
	return PrecisionConfig{
		IntermediatePrecision: 16,
		OutputPrecision:       8,
		Rounding:              RoundHalfUp,
	}
}

// Pre-computed constants to avoid repeated allocations.
var (
	Zero = decimal.NewFromInt(0)
	One  = decimal.NewFromInt(1)
)

// NewDecimal parses a decimal number from its string representation.
// Returns decimal.Zero if the string is invalid.
func NewDecimal(s string) Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return Zero
	}
	return d
}

// NewDecimalFromInt creates a Decimal from an int64.
func NewDecimalFromInt(i int64) Decimal {
	return decimal.NewFromInt(i)
}

// RoundOutput rounds a value to the output precision using the configured
// rounding mode.
func (pc PrecisionConfig) RoundOutput(d Decimal) Decimal {
	switch pc.Rounding {
	case RoundDown:
		return d.Truncate(pc.OutputPrecision)
	case RoundUp:
		return d.RoundCeil(pc.OutputPrecision)
	case RoundBankers:
		return d.RoundBank(pc.OutputPrecision)
	case RoundHalfDown:
		// shopspring/decimal does not have a native half-down; use banker's
		// as the closest built-in behaviour.
		return d.RoundBank(pc.OutputPrecision)
	default: // RoundHalfUp
		return d.Round(pc.OutputPrecision)
	}
}
