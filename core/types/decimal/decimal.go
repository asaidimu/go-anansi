// Package decimal provides arbitrary-precision fixed-point decimal arithmetic
// tailored for monetary and financial applications.
//
// All Decimal values are immutable and safe for concurrent use across goroutines.
// The zero value of a Decimal is valid and represents 0 with a scale of 0.
package decimal

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Common errors returned by decimal operations.
var (
	ErrZeroDivision  = errors.New("decimal: division by zero")
	ErrInvalidFormat = errors.New("decimal: invalid decimal string format")
)

// RoundingMode defines how fractional values are rounded when rescaling or dividing.
type RoundingMode int

const (
	// RoundHalfEven (Banker's Rounding) rounds towards the nearest neighbor.
	// If equidistant, it rounds towards the even neighbor. This is the default standard for finance.
	RoundHalfEven RoundingMode = iota

	// RoundHalfUp rounds towards the nearest neighbor. If equidistant, it rounds away from zero.
	RoundHalfUp

	// RoundTruncate drops excess decimal places towards zero.
	RoundTruncate
)

// Decimal represents an immutable fixed-point decimal value expressed as:
//
//	Value * 10^-Scale
//
// Unexported fields guarantee immutability and thread safety by preventing pointer aliasing.
type Decimal struct {
	value *big.Int // unscaled integer representation
	scale int32    // number of digits to the right of the decimal point
}

var (
	zeroInt = big.NewInt(0)
	tenInt  = big.NewInt(10)
)

// New creates a Decimal from an unscaled int64 integer and a scale.
// For example, New(1234, 2) represents 12.34.
func New(unscaled int64, scale int32) Decimal {
	return Decimal{
		value: big.NewInt(unscaled),
		scale: scale,
	}
}

// NewFromBigInt creates a Decimal from a *big.Int and a scale.
// The provided *big.Int is copied internally to preserve immutability.
func NewFromBigInt(val *big.Int, scale int32) Decimal {
	if val == nil {
		return Decimal{value: big.NewInt(0), scale: scale}
	}
	return Decimal{
		value: new(big.Int).Set(val),
		scale: scale,
	}
}

// NewFromString parses a string representation of a decimal number (e.g., "123.45", "-0.001", "42").
func NewFromString(s string) (Decimal, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return Decimal{}, fmt.Errorf("%w: empty string", ErrInvalidFormat)
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return Decimal{}, fmt.Errorf("%w: multiple decimal points in %q", ErrInvalidFormat, s)
	}

	if len(parts) == 1 {
		val, ok := new(big.Int).SetString(parts[0], 10)
		if !ok {
			return Decimal{}, fmt.Errorf("%w: invalid integer %q", ErrInvalidFormat, parts[0])
		}
		return Decimal{value: val, scale: 0}, nil
	}

	fracStr := parts[1]
	scale := int32(len(fracStr))

	rawStr := parts[0] + fracStr
	val, ok := new(big.Int).SetString(rawStr, 10)
	if !ok {
		return Decimal{}, fmt.Errorf("%w: invalid number string %q", ErrInvalidFormat, s)
	}

	return Decimal{value: val, scale: scale}, nil
}

// --- Accessors & Inspection ---

// rawValue returns the underlying big.Int safely, coping with zero-value structs.
func (d Decimal) rawValue() *big.Int {
	if d.value == nil {
		return zeroInt
	}
	return d.value
}

// Scale returns the number of fractional digits after the decimal point.
func (d Decimal) Scale() int32 {
	return d.scale
}

// Sign returns:
//
//	-1 if d < 0
//	 0 if d == 0
//	+1 if d > 0
func (d Decimal) Sign() int {
	return d.rawValue().Sign()
}

// IsZero reports whether d is equal to 0.
func (d Decimal) IsZero() bool {
	return d.Sign() == 0
}

// --- Arithmetic Operations ---

// Add returns d + other, automatically aligning their scales.
func (d Decimal) Add(other Decimal) Decimal {
	a, b, scale := alignScales(d, other)
	res := new(big.Int).Add(a, b)
	return Decimal{value: res, scale: scale}
}

// Sub returns d - other, automatically aligning their scales.
func (d Decimal) Sub(other Decimal) Decimal {
	a, b, scale := alignScales(d, other)
	res := new(big.Int).Sub(a, b)
	return Decimal{value: res, scale: scale}
}

// Mul returns d * other. The resulting scale is equal to d.Scale() + other.Scale().
func (d Decimal) Mul(other Decimal) Decimal {
	res := new(big.Int).Mul(d.rawValue(), other.rawValue())
	return Decimal{value: res, scale: d.scale + other.scale}
}

// Div divides d by other and rounds the result to targetScale using RoundHalfEven (Banker's Rounding).
// Returns ErrZeroDivision if other is zero.
func (d Decimal) Div(other Decimal, targetScale int32) (Decimal, error) {
	if other.IsZero() {
		return Decimal{}, ErrZeroDivision
	}

	// Add internal buffer digits for intermediate accuracy before rounding
	calcScale := targetScale + 2
	shift := calcScale + other.scale - d.scale

	var scaledDividend *big.Int
	if shift >= 0 {
		scaledDividend = scaleUp(d.rawValue(), shift)
	} else {
		divisor := new(big.Int).Exp(tenInt, big.NewInt(int64(-shift)), nil)
		scaledDividend = new(big.Int).Quo(d.rawValue(), divisor)
	}

	rawQuotient := new(big.Int).Quo(scaledDividend, other.rawValue())
	intermediate := Decimal{value: rawQuotient, scale: calcScale}

	return intermediate.Round(targetScale, RoundHalfEven), nil
}

// --- Comparison Operations ---

// Cmp compares d and other and returns:
//
//	-1 if d < other
//	 0 if d == other
//	+1 if d > other
func (d Decimal) Cmp(other Decimal) int {
	a, b, _ := alignScales(d, other)
	return a.Cmp(b)
}

// Equal reports whether d and other represent the same numerical value, regardless of scale.
func (d Decimal) Equal(other Decimal) bool {
	return d.Cmp(other) == 0
}

// --- Rounding & Rescaling ---

// Rescale changes the decimal's scale to targetScale, applying Banker's Rounding (RoundHalfEven).
func (d Decimal) Rescale(targetScale int32) Decimal {
	return d.Round(targetScale, RoundHalfEven)
}

// Round rounds the decimal to targetScale according to the provided RoundingMode.
func (d Decimal) Round(targetScale int32, mode RoundingMode) Decimal {
	if targetScale >= d.scale {
		return Decimal{
			value: scaleUp(d.rawValue(), targetScale-d.scale),
			scale: targetScale,
		}
	}

	shift := d.scale - targetScale
	divisor := new(big.Int).Exp(tenInt, big.NewInt(int64(shift)), nil)

	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(d.rawValue(), divisor, remainder)

	if remainder.Sign() == 0 {
		return Decimal{value: quotient, scale: targetScale}
	}

	switch mode {
	case RoundTruncate:
		// QuoRem truncates toward zero by default

	case RoundHalfUp:
		half := new(big.Int).Div(divisor, big.NewInt(2))
		if new(big.Int).Abs(remainder).Cmp(half) >= 0 {
			if d.rawValue().Sign() >= 0 {
				quotient.Add(quotient, big.NewInt(1))
			} else {
				quotient.Sub(quotient, big.NewInt(1))
			}
		}

	case RoundHalfEven:
		half := new(big.Int).Div(divisor, big.NewInt(2))
		absRem := new(big.Int).Abs(remainder)
		cmp := absRem.Cmp(half)

		if cmp > 0 {
			if d.rawValue().Sign() >= 0 {
				quotient.Add(quotient, big.NewInt(1))
			} else {
				quotient.Sub(quotient, big.NewInt(1))
			}
		} else if cmp == 0 {
			// Round to nearest even quotient
			isOdd := new(big.Int).And(quotient, big.NewInt(1)).Cmp(zeroInt) != 0
			if isOdd {
				if d.rawValue().Sign() >= 0 {
					quotient.Add(quotient, big.NewInt(1))
				} else {
					quotient.Sub(quotient, big.NewInt(1))
				}
			}
		}
	}

	return Decimal{value: quotient, scale: targetScale}
}

// --- Formatting ---

// String converts the Decimal into its standard string representation (e.g., "12.34").
func (d Decimal) String() string {
	val := d.rawValue()
	if d.scale <= 0 {
		if d.scale == 0 {
			return val.String()
		}
		return val.String() + strings.Repeat("0", int(-d.scale))
	}

	sign := ""
	absVal := new(big.Int).Abs(val)
	if val.Sign() < 0 {
		sign = "-"
	}

	str := absVal.String()
	scale := int(d.scale)

	if len(str) <= scale {
		str = strings.Repeat("0", scale-len(str)+1) + str
	}

	idx := len(str) - scale
	return sign + str[:idx] + "." + str[idx:]
}

// --- Internal Helpers ---

func alignScales(a, b Decimal) (*big.Int, *big.Int, int32) {
	targetScale := max(b.scale, a.scale)

	aScaled := scaleUp(a.rawValue(), targetScale-a.scale)
	bScaled := scaleUp(b.rawValue(), targetScale-b.scale)

	return aScaled, bScaled, targetScale
}

func scaleUp(val *big.Int, factor int32) *big.Int {
	if factor == 0 {
		return new(big.Int).Set(val)
	}
	multiplier := new(big.Int).Exp(tenInt, big.NewInt(int64(factor)), nil)
	return new(big.Int).Mul(val, multiplier)
}

// IsValid reports whether s is a valid decimal string representation (e.g., "123.45", "-0.01", "42").
func IsValid(value any) bool {
	if s, ok := value.(string); ok {
		_, err := NewFromString(s)
		return err == nil
	}
	return false
}

// Validate checks if a string is a valid decimal representation.
// It returns true if valid, false otherwise.
func Validate(s string) bool {
	return IsValid(s)
}

// --- Value-Level Validation (Business Rules) ---

// ValidationRules defines business constraints for monetary validation.
type ValidationRules struct {
	MaxScale    *int32 // Optional: Max allowed decimal places (e.g., 2 for standard currency)
	NonNegative bool   // Optional: Must be >= 0
	Positive    bool   // Optional: Must be > 0
}

// ValidateRules checks whether an instantiated Decimal complies with business rules.
func (d Decimal) ValidateRules(rules ValidationRules) bool {
	if rules.NonNegative && d.Sign() < 0 {
		return false
	}
	if rules.Positive && d.Sign() <= 0 {
		return false
	}
	if rules.MaxScale != nil && d.Scale() > *rules.MaxScale {
		return false
	}
	return true
}
