#!/usr/bin/env python3
"""Compile the server-owned Go harness, or emit a compile-error verdict."""

from __future__ import annotations

import json
import os
import pathlib
import subprocess


PROTOCOL_DIR = pathlib.Path(".codegym")
VERDICT_PATH = PROTOCOL_DIR / "verdict.json"
BINARY_PATH = PROTOCOL_DIR / "submission"
COMPILE_ERROR_CAP_BYTES = 16 * 1024
SOURCES = ("solution.go", "codegym_comparator.go", "test_solution.go")


def write_compile_error(message: str) -> None:
    VERDICT_PATH.write_text(
        json.dumps(
            {"schema": 1, "status": "failed", "compile_error": message, "cases": []},
            separators=(",", ":"),
        ),
        encoding="utf-8",
    )


def truncate(data: bytes) -> str:
    if len(data) <= COMPILE_ERROR_CAP_BYTES:
        return data.decode("utf-8", errors="replace")
    suffix = b"\n...[compile error truncated]"
    return (data[: COMPILE_ERROR_CAP_BYTES - len(suffix)] + suffix).decode("utf-8", errors="replace")


def main() -> int:
    PROTOCOL_DIR.mkdir(exist_ok=True)
    try:
        sources = list(SOURCES)
        if pathlib.Path("checker.go").exists():
            sources.append("checker.go")
        completed = subprocess.run(
            ["go", "build", "-o", str(BINARY_PATH), *sources],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
    except Exception as exc:
        write_compile_error(f"{type(exc).__name__}: {exc}")
        return 0
    if completed.returncode != 0:
        detail = completed.stderr or completed.stdout or b"go build failed without diagnostic output"
        write_compile_error(truncate(detail))
        return 0
    os.execv(str(BINARY_PATH), [str(BINARY_PATH)])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
