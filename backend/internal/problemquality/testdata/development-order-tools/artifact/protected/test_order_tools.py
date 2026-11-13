import importlib.util
import pathlib
import sys

PROTOCOL = "CODEGYM_TEST"


def emit(test_id, passed, code="", expected="", actual=""):
    if passed:
        print(f"{PROTOCOL} {test_id} PASS")
        return
    print(f"{PROTOCOL} {test_id} FAIL code={code} expected={expected} actual={actual}")


def load_solution():
    path = pathlib.Path(__file__).resolve().parent.parent / "solution.py"
    spec = importlib.util.spec_from_file_location("solution", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main():
    try:
        solution = load_solution()
        normalize = solution.normalize_tags
    except Exception as exc:
        emit("load-solution", False, "LOAD", "solution", type(exc).__name__)
        return 1

    # Comparator: length plus set equality. Different valid orders pass.
    try:
        got = normalize(["beta", "alpha", "beta"])
        passed = len(got) == 2 and set(got) == {"alpha", "beta"}
        emit("deduplicates", passed, "DEDUPE", "2", str(len(got)))
    except Exception as exc:
        emit("deduplicates", False, "DEDUPE", "2", type(exc).__name__)

    try:
        got = normalize([])
        passed = list(got) == []
        emit("empty-input", passed, "EMPTY", "0", str(len(got)))
    except Exception as exc:
        emit("empty-input", False, "EMPTY", "0", type(exc).__name__)

    # Comparator: set equality only, so descending reference order also passes.
    try:
        got = normalize(["gamma", "alpha", "beta"])
        passed = set(got) == {"alpha", "beta", "gamma"} and len(got) == 3
        emit("order-insensitive", passed, "ORDER", "alpha,beta,gamma", ",".join(got))
    except Exception as exc:
        emit("order-insensitive", False, "ORDER", "alpha,beta,gamma", type(exc).__name__)

    # Intentionally weak: only lowercase input is exercised. A case-sensitive
    # implementation survives, exposing the missing uppercase coverage.
    try:
        got = normalize(["alpha"])
        passed = len(got) == 1 and got[0] == "alpha"
        emit("normalization", passed, "CASE", "alpha", got[0] if got else "empty")
    except Exception as exc:
        emit("normalization", False, "CASE", "alpha", type(exc).__name__)

    return 0


if __name__ == "__main__":
    sys.exit(main())
