package test

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type testAssertions struct {
	t testing.TB
}

var numericCmpOptions = []cmp.Option{
	cmp.FilterValues(
		func(x, y any) bool {
			return isNumeric(x) && isNumeric(y)
		},
		cmp.Comparer(func(x, y any) bool {
			rx, okX := toBigRat(x)
			if !okX {
				return false
			}
			ry, okY := toBigRat(y)
			if !okY {
				return false
			}
			return rx.Cmp(ry) == 0
		}),
	),
}

func newAssertions(tb testing.TB) *testAssertions {
	tb.Helper()
	return &testAssertions{t: tb}
}

func (a *testAssertions) helper() {
	a.t.Helper()
}

func (a *testAssertions) NoError(err error, msgAndArgs ...any) {
	a.helper()
	if err == nil {
		return
	}
	a.fail(fmt.Sprintf("unexpected error: %v", err), msgAndArgs...)
}

func (a *testAssertions) Error(err error, msgAndArgs ...any) {
	a.helper()
	if err != nil {
		return
	}
	a.fail("expected an error", msgAndArgs...)
}

func (a *testAssertions) True(condition bool, msgAndArgs ...any) {
	a.helper()
	if condition {
		return
	}
	a.fail("expected condition to be true", msgAndArgs...)
}

func (a *testAssertions) False(condition bool, msgAndArgs ...any) {
	a.helper()
	if !condition {
		return
	}
	a.fail("expected condition to be false", msgAndArgs...)
}

func (a *testAssertions) Equal(expected, actual any, msgAndArgs ...any) {
	a.helper()
	diff := cmp.Diff(expected, actual)
	if diff == "" {
		return
	}
	a.fail("values are not equal (-want +got):\n"+diff, msgAndArgs...)
}

func (a *testAssertions) EqualValues(expected, actual any, msgAndArgs ...any) {
	a.helper()
	a.Equal(expected, actual, msgAndArgs...)
}

func (a *testAssertions) EqualNumericValues(expected, actual any, msgAndArgs ...any) {
	a.helper()
	if !isNumeric(expected) || !isNumeric(actual) {
		a.fail("EqualNumericValues requires numeric inputs", msgAndArgs...)
		return
	}
	diff := cmp.Diff(expected, actual, numericCmpOptions...)
	if diff == "" {
		return
	}
	a.fail("numeric values are not equal (-want +got):\n"+diff, msgAndArgs...)
}

func (a *testAssertions) EqualError(err error, expected string, msgAndArgs ...any) {
	a.helper()
	if err == nil {
		a.fail("expected error but got nil", msgAndArgs...)
		return
	}
	diff := cmp.Diff(expected, err.Error())
	if diff == "" {
		return
	}
	a.fail("unexpected error message (-want +got):\n"+diff, msgAndArgs...)
}

func (a *testAssertions) InDelta(expected, actual any, delta float64, msgAndArgs ...any) {
	a.helper()
	expectedFloat, ok := toFloat64(expected)
	if !ok {
		a.fail(fmt.Sprintf("expected value %v is not numeric", expected), msgAndArgs...)
		return
	}
	actualFloat, ok := toFloat64(actual)
	if !ok {
		a.fail(fmt.Sprintf("actual value %v is not numeric", actual), msgAndArgs...)
		return
	}
	if math.Abs(expectedFloat-actualFloat) <= delta {
		return
	}
	a.fail(fmt.Sprintf("difference %v exceeds delta %v", math.Abs(expectedFloat-actualFloat), delta), msgAndArgs...)
}

func (a *testAssertions) Len(value any, length int, msgAndArgs ...any) {
	a.helper()
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Array, reflect.Slice, reflect.Map, reflect.String, reflect.Chan:
		// allowed collection types
	default:
		a.fail(fmt.Sprintf("unsupported type %T for Len assertion", value), msgAndArgs...)
		return
	}
	if v.Len() == length {
		return
	}
	a.fail(fmt.Sprintf("unexpected length: expected %d, got %d", length, v.Len()), msgAndArgs...)
}

func (a *testAssertions) NotEmpty(value any, msgAndArgs ...any) {
	a.helper()
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Array, reflect.Slice, reflect.Map, reflect.Chan, reflect.String:
		if v.Len() > 0 {
			return
		}
		a.fail("expected value to be non-empty", msgAndArgs...)
	default:
		if !isZero(value) {
			return
		}
		a.fail("expected value to be non-zero", msgAndArgs...)
	}
}

func (a *testAssertions) GreaterOrEqual(value, minimum int, msgAndArgs ...any) {
	a.helper()
	if value >= minimum {
		return
	}
	a.fail(fmt.Sprintf("expected %d >= %d", value, minimum), msgAndArgs...)
}

func (a *testAssertions) fail(defaultMsg string, msgAndArgs ...any) {
	a.helper()
	if len(msgAndArgs) == 0 {
		a.t.Fatal(defaultMsg)
		return
	}
	if format, ok := msgAndArgs[0].(string); ok {
		if len(msgAndArgs) == 1 {
			a.t.Fatal(format)
			return
		}
		a.t.Fatalf(format, msgAndArgs[1:]...)
		return
	}
	a.t.Fatalf("%s: %v", defaultMsg, msgAndArgs)
}

func toFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	default:
		return 0, false
	}
}

func isZero(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	if v.IsValid() {
		zero := reflect.Zero(v.Type()).Interface()
		return reflect.DeepEqual(value, zero)
	}
	return true
}

func requireNew(tb testing.TB) *testAssertions {
	tb.Helper()
	return newAssertions(tb)
}

func requireNoError(tb testing.TB, err error, msgAndArgs ...any) {
	tb.Helper()
	newAssertions(tb).NoError(err, msgAndArgs...)
}

func assertNoError(tb testing.TB, err error, msgAndArgs ...any) {
	tb.Helper()
	newAssertions(tb).NoError(err, msgAndArgs...)
}

func assertError(tb testing.TB, err error, msgAndArgs ...any) {
	tb.Helper()
	newAssertions(tb).Error(err, msgAndArgs...)
}

func assertEqualValues(tb testing.TB, expected, actual any, msgAndArgs ...any) {
	tb.Helper()
	newAssertions(tb).EqualValues(expected, actual, msgAndArgs...)
}

func assertEqualNumericValues(tb testing.TB, expected, actual any, msgAndArgs ...any) {
	tb.Helper()
	newAssertions(tb).EqualNumericValues(expected, actual, msgAndArgs...)
}

func isNumeric(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		*big.Int, big.Int,
		*big.Rat, big.Rat:
		return true
	default:
		return false
	}
}

func toBigRat(value any) (*big.Rat, bool) {
	switch v := value.(type) {
	case int:
		return new(big.Rat).SetInt64(int64(v)), true
	case int8:
		return new(big.Rat).SetInt64(int64(v)), true
	case int16:
		return new(big.Rat).SetInt64(int64(v)), true
	case int32:
		return new(big.Rat).SetInt64(int64(v)), true
	case int64:
		return new(big.Rat).SetInt64(v), true
	case uint:
		return ratFromUint64(uint64(v)), true
	case uint8:
		return ratFromUint64(uint64(v)), true
	case uint16:
		return ratFromUint64(uint64(v)), true
	case uint32:
		return ratFromUint64(uint64(v)), true
	case uint64:
		return ratFromUint64(v), true
	case float32:
		return ratFromFloat(float64(v))
	case float64:
		return ratFromFloat(v)
	case big.Int:
		return new(big.Rat).SetInt(&v), true
	case *big.Int:
		if v == nil {
			return nil, false
		}
		return new(big.Rat).SetInt(v), true
	case big.Rat:
		return new(big.Rat).Set(&v), true
	case *big.Rat:
		if v == nil {
			return nil, false
		}
		return new(big.Rat).Set(v), true
	default:
		return nil, false
	}
}

func ratFromUint64(v uint64) *big.Rat {
	return new(big.Rat).SetInt(new(big.Int).SetUint64(v))
}

func ratFromFloat(v float64) (*big.Rat, bool) {
	rat := new(big.Rat)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, false
	}
	rat.SetFloat64(v)
	return rat, true
}
