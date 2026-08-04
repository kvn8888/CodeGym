#!/usr/bin/env python3
"""Run an untrusted child and always write .codegym/result.json.

The script is intentionally language-agnostic. It is embedded in the backend
binary and uploaded for every run. If a future Daytona stack does not include
Python 3, the upgrade path is a small static Go supervisor with this protocol.
"""

from __future__ import annotations

import argparse
import json
import os
import resource
import signal
import subprocess
import sys
import threading
import time
from pathlib import Path
from typing import Any


SCHEMA = 1
RESULT_PREFIX = "CODEGYM_RESULT "
FINAL_STATUSES = {"passed", "failed"}
MEMORY_MARKERS = (
    b"memoryerror",
    b"cannot allocate memory",
    b"out of memory",
    b"std::bad_alloc",
    b"allocation failed",
)


class CappedBuffer:
    def __init__(self, cap: int) -> None:
        self._cap = cap
        self._data = bytearray()
        self._scan_tail = b""
        self.truncated = False
        self.saw_memory_error = False

    def consume(self, stream: Any) -> None:
        try:
            while True:
                chunk = stream.read(65536)
                if not chunk:
                    return
                scan = self._scan_tail + chunk.lower()
                if any(marker in scan for marker in MEMORY_MARKERS):
                    self.saw_memory_error = True
                self._scan_tail = scan[-64:]
                remaining = self._cap - len(self._data)
                if remaining > 0:
                    self._data.extend(chunk[:remaining])
                if len(chunk) > remaining:
                    self.truncated = True
        finally:
            stream.close()

    def text(self) -> str:
        return bytes(self._data).decode("utf-8", errors="replace")


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work-dir", default=".codegym")
    parser.add_argument("--timeout-seconds", required=True, type=float)
    parser.add_argument("--memory-mb", required=True, type=int)
    parser.add_argument("--output-cap-bytes", required=True, type=int)
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args(argv)
    if args.command and args.command[0] == "--":
        args.command = args.command[1:]
    if not args.command:
        raise ValueError("child command is required")
    if args.timeout_seconds <= 0:
        raise ValueError("timeout must be positive")
    if args.memory_mb <= 0:
        raise ValueError("memory limit must be positive")
    if args.output_cap_bytes <= 0:
        raise ValueError("output cap must be positive")
    return args


def child_setup(memory_bytes: int) -> None:
    os.setsid()
    # Daytona sandboxes are Linux. macOS reports RLIMIT_AS but the framework
    # Python used by local Go tests rejects every finite value; keep the local
    # process-control tests runnable without pretending that is production.
    if sys.platform == "darwin":
        return
    # The soft ceiling is sufficient and applies only to the child. Keeping the
    # inherited hard limit avoids making the ceiling irreversible inside exec.
    _, hard_limit = resource.getrlimit(resource.RLIMIT_AS)
    resource.setrlimit(resource.RLIMIT_AS, (memory_bytes, hard_limit))


def read_json(path: Path) -> tuple[Any | None, str | None]:
    if not path.exists():
        return None, None
    try:
        return json.loads(path.read_text(encoding="utf-8")), None
    except Exception as exc:  # malformed harness output must not kill the parent
        return None, f"could not parse {path.name}: {type(exc).__name__}: {exc}"


def read_progress(path: Path) -> tuple[list[dict[str, Any]], str | None]:
    events: list[dict[str, Any]] = []
    if not path.exists():
        return events, None
    malformed: list[int] = []
    try:
        with path.open("r", encoding="utf-8") as handle:
            for line_number, line in enumerate(handle, start=1):
                if not line.strip():
                    continue
                try:
                    event = json.loads(line)
                    if not isinstance(event, dict):
                        raise ValueError("event is not an object")
                    events.append(event)
                except Exception:
                    malformed.append(line_number)
    except Exception as exc:
        return events, f"could not read cases.jsonl: {type(exc).__name__}: {exc}"
    if malformed:
        return events, "malformed cases.jsonl line(s): " + ", ".join(map(str, malformed))
    return events, None


def progress_cases(events: list[dict[str, Any]]) -> tuple[list[dict[str, Any]], str | None]:
    cases: list[dict[str, Any]] = []
    in_flight: str | None = None
    for event in events:
        event_type = event.get("event")
        name = event.get("name")
        if not isinstance(name, str) or not name:
            continue
        if event_type == "case_start":
            in_flight = name
            continue
        if event_type != "case_result" or event.get("status") not in {"pass", "fail"}:
            continue
        duration_ms = event.get("duration_ms", 0)
        if not isinstance(duration_ms, int) or duration_ms < 0:
            duration_ms = 0
        error = event.get("error")
        if error is not None and not isinstance(error, str):
            error = str(error)
        cases.append(
            {
                "name": name,
                "status": event["status"],
                "duration_ms": duration_ms,
                "error": error,
            }
        )
        if in_flight == name:
            in_flight = None
    return cases, in_flight


def normalized_verdict(payload: Any) -> dict[str, Any] | None:
    if not isinstance(payload, dict) or payload.get("schema") != SCHEMA:
        return None
    if payload.get("status") not in FINAL_STATUSES:
        return None
    cases = payload.get("cases")
    if not isinstance(cases, list):
        return None
    normalized_cases: list[dict[str, Any]] = []
    for case in cases:
        if not isinstance(case, dict):
            return None
        if not isinstance(case.get("name"), str) or case.get("status") not in {"pass", "fail"}:
            return None
        duration_ms = case.get("duration_ms")
        if not isinstance(duration_ms, int) or duration_ms < 0:
            return None
        error = case.get("error")
        if error is not None and not isinstance(error, str):
            return None
        normalized_cases.append(
            {
                "name": case["name"],
                "status": case["status"],
                "duration_ms": duration_ms,
                "error": error,
            }
        )
    compile_error = payload.get("compile_error")
    if compile_error is not None and not isinstance(compile_error, str):
        return None
    return {
        "status": payload["status"],
        "cases": normalized_cases,
        "compile_error": compile_error,
    }


def legacy_verdict(stdout: str) -> dict[str, Any] | None:
    for line in reversed(stdout.rstrip("\r\n").splitlines()):
        if not line.startswith(RESULT_PREFIX):
            continue
        try:
            payload = json.loads(line[len(RESULT_PREFIX) :])
        except Exception:
            return None
        tests = payload.get("tests") if isinstance(payload, dict) else None
        compile_error = payload.get("compile_error") if isinstance(payload, dict) else None
        if not isinstance(tests, list) or (not tests and compile_error is None):
            return None
        cases: list[dict[str, Any]] = []
        failed = compile_error is not None
        for test in tests:
            if not isinstance(test, dict) or test.get("status") not in {"pass", "fail"}:
                return None
            duration_ms = test.get("duration_ms", 0)
            if not isinstance(duration_ms, int) or duration_ms < 0:
                return None
            failed = failed or test["status"] == "fail"
            cases.append(
                {
                    "name": str(test.get("name", "")),
                    "status": test["status"],
                    "duration_ms": duration_ms,
                    "error": test.get("error"),
                }
            )
        return {
            "status": "failed" if failed else "passed",
            "cases": cases,
            "compile_error": compile_error,
        }
    return None


def death_detail(status: str, duration_ms: int, in_flight: str | None, signal_name: str | None) -> str:
    if status == "timeout":
        detail = f"timed out after {duration_ms}ms"
    elif status == "out_of_memory":
        detail = "memory limit exceeded"
    elif signal_name:
        detail = f"submission terminated by {signal_name}"
    else:
        detail = "submission exited without a verdict"
    if in_flight:
        detail += f" during case '{in_flight}'"
    return detail


def classify_death(timed_out: bool, returncode: int, saw_memory_error: bool) -> tuple[str, str | None]:
    signal_name = None
    if returncode < 0:
        try:
            signal_name = signal.Signals(-returncode).name
        except ValueError:
            signal_name = f"SIG{-returncode}"
    if timed_out:
        return "timeout", signal_name
    if signal_name == "SIGALRM":
        return "timeout", signal_name
    if signal_name == "SIGKILL" or saw_memory_error:
        return "out_of_memory", signal_name
    return "crashed", signal_name


def write_result(work_dir: Path, result: dict[str, Any]) -> None:
    work_dir.mkdir(parents=True, exist_ok=True)
    target = work_dir / "result.json"
    temporary = work_dir / "result.json.tmp"
    temporary.write_text(json.dumps(result, separators=(",", ":")), encoding="utf-8")
    os.replace(temporary, target)


def empty_crash(detail: str) -> dict[str, Any]:
    return {
        "schema": SCHEMA,
        "status": "crashed",
        "cases": [],
        "compile_error": None,
        "failure_detail": detail,
        "exit_code": None,
        "signal": None,
        "duration_ms": 0,
        "stdout": "",
        "stderr": "",
        "output_truncated": False,
    }


def run(args: argparse.Namespace) -> dict[str, Any]:
    work_dir = Path(args.work_dir)
    work_dir.mkdir(parents=True, exist_ok=True)
    for name in ("cases.jsonl", "verdict.json", "result.json", "result.json.tmp"):
        try:
            (work_dir / name).unlink()
        except FileNotFoundError:
            pass

    stdout_buffer = CappedBuffer(args.output_cap_bytes)
    stderr_buffer = CappedBuffer(args.output_cap_bytes)
    started = time.monotonic()
    process = subprocess.Popen(
        args.command,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        preexec_fn=lambda: child_setup(args.memory_mb * 1024 * 1024),
    )
    assert process.stdout is not None and process.stderr is not None
    readers = [
        threading.Thread(target=stdout_buffer.consume, args=(process.stdout,), daemon=True),
        threading.Thread(target=stderr_buffer.consume, args=(process.stderr,), daemon=True),
    ]
    for reader in readers:
        reader.start()

    timed_out = False
    try:
        process.wait(timeout=args.timeout_seconds)
    except subprocess.TimeoutExpired:
        timed_out = True
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.wait()
    for reader in readers:
        reader.join(timeout=5)
    duration_ms = max(0, int((time.monotonic() - started) * 1000))
    stdout = stdout_buffer.text()
    stderr = stderr_buffer.text()

    events, progress_error = read_progress(work_dir / "cases.jsonl")
    cases, in_flight = progress_cases(events)
    verdict_payload, verdict_error = read_json(work_dir / "verdict.json")
    verdict = normalized_verdict(verdict_payload)
    if verdict is None:
        verdict = legacy_verdict(stdout)

    signal_name = None
    if process.returncode < 0:
        try:
            signal_name = signal.Signals(-process.returncode).name
        except ValueError:
            signal_name = f"SIG{-process.returncode}"
    exit_code = process.returncode if process.returncode >= 0 else None

    if verdict is not None:
        status = verdict["status"]
        cases = verdict["cases"]
        compile_error = verdict["compile_error"]
        failure_detail = None
    else:
        status, signal_name = classify_death(timed_out, process.returncode, stderr_buffer.saw_memory_error)
        compile_error = None
        failure_detail = death_detail(status, duration_ms, in_flight, signal_name)
        protocol_errors = [error for error in (verdict_error, progress_error) if error]
        if protocol_errors:
            failure_detail += "; " + "; ".join(protocol_errors)

    return {
        "schema": SCHEMA,
        "status": status,
        "cases": cases,
        "compile_error": compile_error,
        "failure_detail": failure_detail,
        "exit_code": exit_code,
        "signal": signal_name,
        "duration_ms": duration_ms,
        "stdout": stdout,
        "stderr": stderr,
        "output_truncated": stdout_buffer.truncated or stderr_buffer.truncated,
    }


def discover_work_dir(argv: list[str]) -> Path:
    try:
        index = argv.index("--work-dir")
        return Path(argv[index + 1])
    except (ValueError, IndexError):
        return Path(".codegym")


def main() -> int:
    work_dir = discover_work_dir(sys.argv[1:])
    try:
        args = parse_args(sys.argv[1:])
        work_dir = Path(args.work_dir)
        result = run(args)
    except BaseException as exc:  # the parent must always leave a backend-readable result
        result = empty_crash(f"supervisor error: {type(exc).__name__}: {exc}")
    try:
        write_result(work_dir, result)
    except BaseException as exc:
        print(f"could not write result.json: {type(exc).__name__}: {exc}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
