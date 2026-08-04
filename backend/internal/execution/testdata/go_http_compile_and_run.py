#!/usr/bin/env python3
"""Compile a Go HTTP server and its server-owned harness, or emit compile_error."""

from __future__ import annotations

import glob
import json
import os
import pathlib
import subprocess


PROTOCOL_DIR = pathlib.Path(".codegym")
VERDICT_PATH = PROTOCOL_DIR / "verdict.json"
SERVER_BINARY = PROTOCOL_DIR / "http_server"
HARNESS_BINARY = PROTOCOL_DIR / "http_harness"
CONFIG_PATH = PROTOCOL_DIR / "http_cases.json"
COMPILE_ERROR_CAP_BYTES = 16 * 1024


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
    return (data[: COMPILE_ERROR_CAP_BYTES - len(suffix)] + suffix).decode(
        "utf-8", errors="replace"
    )


def compile_binary(output: pathlib.Path, sources: list[str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        ["go", "build", "-o", str(output), *sources],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


def main() -> int:
    PROTOCOL_DIR.mkdir(exist_ok=True)
    try:
        server_sources = sorted(glob.glob("*.go"))
        if not server_sources:
            write_compile_error("go build: no Go source files were submitted")
            return 0
        completed = compile_binary(SERVER_BINARY, server_sources)
    except Exception as exc:
        write_compile_error(f"{type(exc).__name__}: {exc}")
        return 0
    if completed.returncode != 0:
        detail = completed.stderr or completed.stdout or b"go build failed without diagnostic output"
        write_compile_error(truncate(detail))
        return 0

    try:
        completed = compile_binary(
            HARNESS_BINARY,
            [str(PROTOCOL_DIR / "http_harness.go"), str(PROTOCOL_DIR / "codegym_comparator.go")],
        )
    except Exception as exc:
        write_compile_error(f"HTTP harness compile failed: {type(exc).__name__}: {exc}")
        return 0
    if completed.returncode != 0:
        detail = completed.stderr or completed.stdout or b"HTTP harness go build failed without diagnostic output"
        write_compile_error("HTTP harness compile failed: " + truncate(detail))
        return 0

    os.execv(
        str(HARNESS_BINARY),
        [str(HARNESS_BINARY), "--server", str(SERVER_BINARY), "--config", str(CONFIG_PATH)],
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
