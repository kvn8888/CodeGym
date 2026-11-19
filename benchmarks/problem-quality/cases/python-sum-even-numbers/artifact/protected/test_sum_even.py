import importlib.util
import pathlib

PROTOCOL = "CODEGYM_TEST"


def emit(test_id, passed, code, expected, actual):
    if passed:
        print(f"\n{PROTOCOL} {test_id} PASS")
        return
    print(f"\n{PROTOCOL} {test_id} FAIL code={code} expected={expected} actual={actual}")


def load_solution():
    path = pathlib.Path(__file__).resolve().parent.parent / "solution.py"
    spec = importlib.util.spec_from_file_location("solution", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def check(test_id, numbers, expected):
    solution = load_solution()
    try:
        actual = solution.sum_even(numbers)
    except Exception as exc:
        emit(test_id, False, "ERROR", str(expected), type(exc).__name__)
        return
    emit(test_id, actual == expected, "WRONG_SUM", str(expected), str(actual))


def test_hidden_mixed_values():
    check("hidden-mixed-values", [1, 2, 3, 4], 6)


def test_hidden_negative_values():
    check("hidden-negative-values", [-2, 2], 0)


def test_hidden_empty_values():
    check("hidden-empty-values", [], 0)


def test_hidden_zero_value():
    check("hidden-zero-value", [0], 0)
