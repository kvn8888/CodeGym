#!/usr/bin/env python3
"""Evaluate comparators.json with the Python harness implementation."""

from __future__ import annotations

import json
import math
import sys
from pathlib import Path
from typing import Any

from comparator_python import compare_values


SPECIAL_NUMBER = "$codegym_number"


def revive(value: Any) -> Any:
    if isinstance(value, list):
        return [revive(item) for item in value]
    if isinstance(value, dict):
        if set(value) == {SPECIAL_NUMBER}:
            return {
                "nan": math.nan,
                "+inf": math.inf,
                "-inf": -math.inf,
            }[value[SPECIAL_NUMBER]]
        return {key: revive(item) for key, item in value.items()}
    return value


def main() -> int:
    if len(sys.argv) != 3:
        raise SystemExit("usage: run_python_comparators.py TABLE OUTPUT")
    rows = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
    results = []
    for index, row in enumerate(rows):
        try:
            equal = compare_values(
                row["kind"],
                revive(row["expected"]),
                revive(row["actual"]),
                row.get("epsilon"),
            )
            results.append({"index": index, "equal": equal, "error": None})
        except Exception as exc:
            results.append({"index": index, "equal": False, "error": f"{type(exc).__name__}: {exc}"})
    Path(sys.argv[2]).write_text(json.dumps(results, separators=(",", ":")), encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
