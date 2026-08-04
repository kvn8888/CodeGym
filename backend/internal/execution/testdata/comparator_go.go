package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const defaultFloatEpsilon = 1e-6

func compareValues(kind string, expected, actual any, epsilon *float64) (bool, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		kind = "exact"
	}
	switch kind {
	case "exact":
		return deepEqual(reflect.ValueOf(expected), reflect.ValueOf(actual)), nil
	case "set":
		return setEqual(reflect.ValueOf(expected), reflect.ValueOf(actual), false), nil
	case "multiset":
		return setEqual(reflect.ValueOf(expected), reflect.ValueOf(actual), true), nil
	case "sorted":
		return sortedEqual(reflect.ValueOf(expected), reflect.ValueOf(actual)), nil
	case "float":
		tolerance := defaultFloatEpsilon
		if epsilon != nil {
			tolerance = *epsilon
		}
		if tolerance <= 0 || math.IsNaN(tolerance) || math.IsInf(tolerance, 0) {
			return false, errors.New("float comparator epsilon must be finite and greater than zero")
		}
		return floatEqual(reflect.ValueOf(expected), reflect.ValueOf(actual), tolerance), nil
	case "checker":
		return false, errors.New("checker comparators are evaluated by the authored check function")
	default:
		return false, fmt.Errorf("unknown comparator kind %q", kind)
	}
}

func unwrap(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
}

func numericValue(value reflect.Value) (float64, bool) {
	value = unwrap(value)
	if !value.IsValid() {
		return 0, false
	}
	if value.Type() == reflect.TypeOf(json.Number("")) {
		number, err := strconv.ParseFloat(value.Interface().(json.Number).String(), 64)
		return number, err == nil
	}
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(value.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(value.Uint()), true
	case reflect.Float32, reflect.Float64:
		return value.Float(), true
	default:
		return 0, false
	}
}

func deepEqual(expected, actual reflect.Value) bool {
	expected = unwrap(expected)
	actual = unwrap(actual)
	if !expected.IsValid() || !actual.IsValid() {
		return !expected.IsValid() && !actual.IsValid()
	}
	if left, ok := numericValue(expected); ok {
		right, rightOK := numericValue(actual)
		if !rightOK || math.IsNaN(left) || math.IsNaN(right) {
			return false
		}
		return left == right
	}
	if _, ok := numericValue(actual); ok {
		return false
	}
	if expected.Kind() == reflect.Bool || actual.Kind() == reflect.Bool {
		return expected.Kind() == reflect.Bool && actual.Kind() == reflect.Bool && expected.Bool() == actual.Bool()
	}
	if expected.Kind() == reflect.String || actual.Kind() == reflect.String {
		return expected.Kind() == reflect.String && actual.Kind() == reflect.String && expected.String() == actual.String()
	}
	if isSequence(expected) && isSequence(actual) {
		if sequenceNilMismatch(expected, actual) || expected.Len() != actual.Len() {
			return false
		}
		for index := 0; index < expected.Len(); index++ {
			if !deepEqual(expected.Index(index), actual.Index(index)) {
				return false
			}
		}
		return true
	}
	if expected.Kind() == reflect.Map && actual.Kind() == reflect.Map {
		return mapsEqual(expected, actual, func(left, right reflect.Value) bool { return deepEqual(left, right) })
	}
	if expected.Type() != actual.Type() || !expected.CanInterface() || !actual.CanInterface() {
		return false
	}
	return reflect.DeepEqual(expected.Interface(), actual.Interface())
}

func isSequence(value reflect.Value) bool {
	value = unwrap(value)
	return value.IsValid() && (value.Kind() == reflect.Slice || value.Kind() == reflect.Array)
}

func sequenceNilMismatch(left, right reflect.Value) bool {
	left = unwrap(left)
	right = unwrap(right)
	leftNil := left.Kind() == reflect.Slice && left.IsNil()
	rightNil := right.Kind() == reflect.Slice && right.IsNil()
	return leftNil != rightNil
}

func setEqual(expected, actual reflect.Value, duplicateSensitive bool) bool {
	expected = unwrap(expected)
	actual = unwrap(actual)
	if !isSequence(expected) || !isSequence(actual) || sequenceNilMismatch(expected, actual) {
		return false
	}
	left := sequenceValues(expected)
	right := sequenceValues(actual)
	if !duplicateSensitive {
		left = uniqueValues(left)
		right = uniqueValues(right)
	}
	if len(left) != len(right) {
		return false
	}
	matched := make([]bool, len(right))
	for _, candidate := range left {
		found := false
		for index, other := range right {
			if !matched[index] && deepEqual(candidate, other) {
				matched[index] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func uniqueValues(values []reflect.Value) []reflect.Value {
	unique := make([]reflect.Value, 0, len(values))
	for _, value := range values {
		seen := false
		for _, prior := range unique {
			if deepEqual(value, prior) {
				seen = true
				break
			}
		}
		if !seen {
			unique = append(unique, value)
		}
	}
	return unique
}

func sortedEqual(expected, actual reflect.Value) bool {
	expected = unwrap(expected)
	actual = unwrap(actual)
	if !isSequence(expected) || !isSequence(actual) || sequenceNilMismatch(expected, actual) || expected.Len() != actual.Len() {
		return false
	}
	left := sequenceValues(expected)
	right := sequenceValues(actual)
	sort.SliceStable(left, func(i, j int) bool { return sortKey(left[i]) < sortKey(left[j]) })
	sort.SliceStable(right, func(i, j int) bool { return sortKey(right[i]) < sortKey(right[j]) })
	for index := range left {
		if !deepEqual(left[index], right[index]) {
			return false
		}
	}
	return true
}

func sequenceValues(value reflect.Value) []reflect.Value {
	value = unwrap(value)
	values := make([]reflect.Value, value.Len())
	for index := 0; index < value.Len(); index++ {
		values[index] = value.Index(index)
	}
	return values
}

func sortKey(value reflect.Value) string {
	value = unwrap(value)
	if !value.IsValid() {
		return "0:"
	}
	if value.Kind() == reflect.Bool {
		return fmt.Sprintf("1:%t", value.Bool())
	}
	if number, ok := numericValue(value); ok {
		if math.IsNaN(number) {
			return "2:nan"
		}
		return "2:" + strconv.FormatFloat(number, 'g', 17, 64)
	}
	if value.Kind() == reflect.String {
		return "3:" + value.String()
	}
	if isSequence(value) {
		parts := make([]string, value.Len())
		for index := 0; index < value.Len(); index++ {
			parts[index] = sortKey(value.Index(index))
		}
		return "4:[" + strings.Join(parts, ",") + "]"
	}
	if value.Kind() == reflect.Map {
		parts := make([]string, 0, value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			parts = append(parts, sortKey(iterator.Key())+"="+sortKey(iterator.Value()))
		}
		sort.Strings(parts)
		return "5:{" + strings.Join(parts, ",") + "}"
	}
	if value.CanInterface() {
		return fmt.Sprintf("6:%T:%v", value.Interface(), value.Interface())
	}
	return "6:" + value.Type().String()
}

func floatEqual(expected, actual reflect.Value, epsilon float64) bool {
	expected = unwrap(expected)
	actual = unwrap(actual)
	if !expected.IsValid() || !actual.IsValid() {
		return !expected.IsValid() && !actual.IsValid()
	}
	if left, ok := numericValue(expected); ok {
		right, rightOK := numericValue(actual)
		if !rightOK || math.IsNaN(left) || math.IsNaN(right) {
			return false
		}
		if math.IsInf(left, 0) || math.IsInf(right, 0) {
			return left == right
		}
		return math.Abs(left-right) <= epsilon
	}
	if _, ok := numericValue(actual); ok {
		return false
	}
	if isSequence(expected) && isSequence(actual) {
		if sequenceNilMismatch(expected, actual) || expected.Len() != actual.Len() {
			return false
		}
		for index := 0; index < expected.Len(); index++ {
			if !floatEqual(expected.Index(index), actual.Index(index), epsilon) {
				return false
			}
		}
		return true
	}
	if expected.Kind() == reflect.Map && actual.Kind() == reflect.Map {
		return mapsEqual(expected, actual, func(left, right reflect.Value) bool { return floatEqual(left, right, epsilon) })
	}
	return deepEqual(expected, actual)
}

func mapsEqual(expected, actual reflect.Value, valueEqual func(reflect.Value, reflect.Value) bool) bool {
	if expected.IsNil() != actual.IsNil() || expected.Len() != actual.Len() {
		return false
	}
	type entry struct{ key, value reflect.Value }
	unmatched := make([]entry, 0, actual.Len())
	iterator := actual.MapRange()
	for iterator.Next() {
		unmatched = append(unmatched, entry{key: iterator.Key(), value: iterator.Value()})
	}
	iterator = expected.MapRange()
	for iterator.Next() {
		found := -1
		for index, candidate := range unmatched {
			if deepEqual(iterator.Key(), candidate.key) && valueEqual(iterator.Value(), candidate.value) {
				found = index
				break
			}
		}
		if found < 0 {
			return false
		}
		unmatched = append(unmatched[:found], unmatched[found+1:]...)
	}
	return len(unmatched) == 0
}
