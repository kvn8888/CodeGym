"""CodeGym's Python acceptance-predicate implementation.

Keep this module dependency-free: it is embedded in generated hidden tests and
also exercised directly by the cross-language conformance suite.
"""

from __future__ import annotations

import math
from collections.abc import Mapping
from typing import Any


DEFAULT_FLOAT_EPSILON = 1e-6


def compare_values(
    kind: str,
    expected: Any,
    actual: Any,
    epsilon: float | None = None,
) -> bool:
    kind = (kind or "exact").strip().lower()
    if kind == "exact":
        return _deep_equal(expected, actual)
    if kind == "set":
        return _set_equal(expected, actual, duplicate_sensitive=False)
    if kind == "multiset":
        return _set_equal(expected, actual, duplicate_sensitive=True)
    if kind == "sorted":
        if not _is_sequence(expected) or not _is_sequence(actual):
            return False
        return _deep_equal(
            sorted(expected, key=_sort_key),
            sorted(actual, key=_sort_key),
        )
    if kind == "float":
        tolerance = DEFAULT_FLOAT_EPSILON if epsilon is None else epsilon
        if not _sane_epsilon(tolerance):
            raise ValueError("float comparator epsilon must be finite and greater than zero")
        return _float_equal(expected, actual, tolerance)
    if kind == "checker":
        raise ValueError("checker comparators are evaluated by the authored check function")
    raise ValueError(f"unknown comparator kind {kind!r}")


def _is_number(value: Any) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def _is_sequence(value: Any) -> bool:
    return isinstance(value, (list, tuple))


def _sane_epsilon(value: Any) -> bool:
    return _is_number(value) and value > 0 and math.isfinite(value)


def _number_equal(expected: int | float, actual: int | float) -> bool:
    if isinstance(expected, float) and math.isnan(expected):
        return False
    if isinstance(actual, float) and math.isnan(actual):
        return False
    return expected == actual


def _deep_equal(expected: Any, actual: Any) -> bool:
    if _is_number(expected) and _is_number(actual):
        return _number_equal(expected, actual)
    if isinstance(expected, bool) or isinstance(actual, bool):
        return type(expected) is type(actual) and expected == actual
    if expected is None or actual is None:
        return expected is None and actual is None
    if _is_sequence(expected) and _is_sequence(actual):
        return len(expected) == len(actual) and all(
            _deep_equal(left, right) for left, right in zip(expected, actual)
        )
    if isinstance(expected, Mapping) and isinstance(actual, Mapping):
        if len(expected) != len(actual):
            return False
        unmatched = list(actual.items())
        for expected_key, expected_value in expected.items():
            for index, (actual_key, actual_value) in enumerate(unmatched):
                if _deep_equal(expected_key, actual_key) and _deep_equal(expected_value, actual_value):
                    unmatched.pop(index)
                    break
            else:
                return False
        return not unmatched
    if type(expected) is not type(actual):
        return False
    return expected == actual


def _set_equal(expected: Any, actual: Any, duplicate_sensitive: bool) -> bool:
    if not _is_sequence(expected) or not _is_sequence(actual):
        return False
    if not duplicate_sensitive:
        expected_unique = _unique(expected)
        actual_unique = _unique(actual)
        return len(expected_unique) == len(actual_unique) and all(
            any(_deep_equal(item, candidate) for candidate in actual_unique)
            for item in expected_unique
        )
    unmatched = list(actual)
    for expected_item in expected:
        for index, actual_item in enumerate(unmatched):
            if _deep_equal(expected_item, actual_item):
                unmatched.pop(index)
                break
        else:
            return False
    return not unmatched


def _unique(values: Any) -> list[Any]:
    unique: list[Any] = []
    for value in values:
        if not any(_deep_equal(value, prior) for prior in unique):
            unique.append(value)
    return unique


def _sort_key(value: Any) -> tuple[Any, ...]:
    if value is None:
        return (0,)
    if isinstance(value, bool):
        return (1, value)
    if _is_number(value):
        if isinstance(value, float) and math.isnan(value):
            return (2, "nan")
        return (2, repr(value))
    if isinstance(value, str):
        return (3, value)
    if _is_sequence(value):
        return (4, tuple(_sort_key(item) for item in value))
    if isinstance(value, Mapping):
        return (5, tuple(sorted((_sort_key(key), _sort_key(item)) for key, item in value.items())))
    return (6, type(value).__name__, repr(value))


def _float_equal(expected: Any, actual: Any, epsilon: float) -> bool:
    if _is_number(expected) and _is_number(actual):
        left = float(expected)
        right = float(actual)
        if math.isnan(left) or math.isnan(right):
            return False
        if math.isinf(left) or math.isinf(right):
            return left == right
        return abs(left - right) <= epsilon
    if isinstance(expected, bool) or isinstance(actual, bool):
        return type(expected) is type(actual) and expected == actual
    if expected is None or actual is None:
        return expected is None and actual is None
    if _is_sequence(expected) and _is_sequence(actual):
        return len(expected) == len(actual) and all(
            _float_equal(left, right, epsilon) for left, right in zip(expected, actual)
        )
    if isinstance(expected, Mapping) and isinstance(actual, Mapping):
        if len(expected) != len(actual):
            return False
        unmatched = list(actual.items())
        for expected_key, expected_value in expected.items():
            for index, (actual_key, actual_value) in enumerate(unmatched):
                if _deep_equal(expected_key, actual_key) and _float_equal(expected_value, actual_value, epsilon):
                    unmatched.pop(index)
                    break
            else:
                return False
        return not unmatched
    return _deep_equal(expected, actual)
