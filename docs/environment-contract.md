# Environment Contract

## Purpose

This contract defines the stable handoff between agent-built environment preparation and backend-owned verification.

It lets Track A implement environment building while Track B implements manifest validation, readiness checks, artifact promotion, and reuse without private knowledge of the builder implementation.

## Core Rule

Builder output never represents readiness.

A successful agent exit, completed runtime task, or natural-language claim is not sufficient for reuse or promotion. An environment is ready only when the backend verifier returns `verdict: "verified"` for a valid manifest and passing backend-controlled checks.

No verifier result means no readiness.
No valid manifest means no promotion.
No passing backend-controlled checks means no promotion.

## Environment Manifest

The environment manifest is the builder-proposed description of the generated exercise environment.

Required top-level fields:

| Field | Owner | Description |
| --- | --- | --- |
| `schemaVersion` | Shared | Contract version, initially `environment.manifest.v1`. |
| `artifact` | Builder proposes, verifier validates | Artifact identity and technology metadata. |
| `environment` | Builder proposes, verifier validates | Runtime base, network mode, dependency files, and lockfiles. |
| `workspace` | Builder proposes, verifier validates | Workspace root, learner-editable files, and protected files. |
| `commands` | Builder proposes, verifier executes | Setup, build, test, and optional run commands. |
| `assessment` | Builder proposes, verifier validates | Visible and hidden assessment artifact references. |
| `compatibility` | Builder proposes, registry validates later | Request metadata and reuse compatibility tags. |

## Builder Result

The builder result describes what the agent/runtime produced before verification.

It must not include `ready`, `verified`, or `promotable` fields.

Required fields:

| Field | Description |
| --- | --- |
| `schemaVersion` | Initially `environment.builder-result.v1`. |
| `builderRunId` | Unique preparation run ID. |
| `runtime` | Runtime name and version. |
| `sandboxRef` | Temporary sandbox reference, if one exists. |
| `artifactRef` | Prepared artifact reference, if one exists before promotion. |
| `proposedManifestPath` | Path to the proposed manifest, or `null` if missing. |
| `diagnostics` | Builder-produced diagnostics. |
| `telemetry` | Runtime, token, cost, wall-time, sandbox, and repair metrics. |

## Verifier Result

The verifier result is the only contract object that can establish readiness.

Required fields:

| Field | Description |
| --- | --- |
| `schemaVersion` | Initially `environment.verifier-result.v1`. |
| `verifierRunId` | Unique verification run ID. |
| `builderRunId` | Builder run being verified. |
| `verdict` | `verified` or `rejected`. |
| `failedStage` | `null` when verified, otherwise the failed stage. |
| `repairable` | Whether diagnostics may be sent back for bounded repair. |
| `diagnostics` | Backend-owned diagnostics. |
| `evidence` | Checks the backend actually ran. |
| `promotion` | Whether registry promotion is allowed and why. |

Allowed failure stages:

| Stage | Meaning |
| --- | --- |
| `manifest` | Missing, malformed, or invalid manifest. |
| `setup` | Dependency setup failed. |
| `build` | Build command failed. |
| `test` | Assessment tests failed. |
| `run` | Runtime command failed. |
| `validation` | Hidden-artifact, editable-file, or contract validation failed. |
| `timeout` | Stage exceeded configured limit. |
| `platform` | Sandbox, filesystem, network, or infrastructure fault. |

## Diagnostics

Diagnostics are shared between builder and verifier.

Required fields:

| Field | Description |
| --- | --- |
| `severity` | `info`, `warning`, or `error`. |
| `stage` | Stage that produced the diagnostic. |
| `code` | Stable machine-readable code. |
| `message` | Human-readable message safe to expose or summarize. |
| `details` | Optional structured details. |

## Promotion Rules

An artifact can be promoted only when all of the following are true:

- The manifest exists.
- The manifest matches the supported schema version.
- The verifier returns `verdict: "verified"`.
- Backend-controlled setup/build/test/validation checks pass.
- Protected files are outside the learner-editable workspace.
- Promotion metadata includes artifact identity, dependency files or lockfiles, setup/build/test commands, validation state, owner/provenance, created-at timestamp, and compatibility rules.

The builder cannot promote an artifact directly.

## Lifecycle Ownership

| Lifecycle Item | Owner |
| --- | --- |
| Create temporary sandbox | Builder track |
| Produce proposed manifest | Builder track |
| Produce builder diagnostics | Builder track |
| Produce builder telemetry | Builder track |
| Validate manifest schema | Verifier track |
| Run setup/build/test checks | Verifier track |
| Decide readiness | Verifier track |
| Decide promotion eligibility | Verifier track |
| Provide repair diagnostics | Verifier track |
| Attempt bounded repair | Builder track |
| Promote artifact | Verifier/registry track |
| Cleanup before verifier handoff | Builder track |
| Cleanup after verifier accepts handoff | Backend/verifier track |

## Contract Examples

Representative examples live in `docs/examples/environment-contract/`.

The examples cover:

- Valid manifest.
- Builder result with missing manifest.
- Malformed manifest.
- Verifier result for failing build.
- Verifier result for passing build.

## Open Decisions

- Adopt or revise the AgentRuntime contract from #172/#173?
- Start with opencode, the purpose-built loop, or both behind one interface?
- What setup permissions are allowed during cold environment creation?
- What production limits apply to cold builds?
- Should Spring Boot fail fast when dependency-cache prerequisites are missing?
- Which exact second technology should be used for the first generic workflow demo?
- How should existing Python/Go saved exercises be migrated or retired?
- What base commit and PR integration path should this epic use?
