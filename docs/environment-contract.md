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

| Field | Owner | Description |
| --- | --- | --- |
| `schemaVersion` | Shared | Initially `environment.builder-result.v1`. |
| `builderRunId` | Builder track | Unique preparation run ID. |
| `runtime` | Builder track | Runtime name and version. |
| `sandboxRef` | Builder track | Temporary sandbox reference, if one exists. |
| `artifactRef` | Builder track | Prepared artifact reference, if one exists before promotion. |
| `proposedManifestPath` | Builder track | Path to the proposed manifest, or `null` if missing. |
| `diagnostics` | Builder track | Builder-produced diagnostics. |
| `telemetry` | Builder track | Runtime, token, cost, wall-time, sandbox, and repair metrics. |

## Verifier Result

The verifier result is the only contract object that can establish readiness.

Required fields:

| Field | Owner | Description |
| --- | --- | --- |
| `schemaVersion` | Shared | Initially `environment.verifier-result.v1`. |
| `verifierRunId` | Verifier track | Unique verification run ID. |
| `builderRunId` | Shared | Builder run being verified. |
| `verdict` | Verifier track | `verified` or `rejected`. |
| `failedStage` | Verifier track | `null` when verified, otherwise the failed stage. |
| `repairable` | Verifier track | Whether diagnostics may be sent back for bounded repair. |
| `diagnostics` | Verifier track | Backend-owned diagnostics. |
| `evidence` | Verifier track | Checks the backend actually ran. |
| `promotion` | Verifier/registry track | Whether registry promotion is allowed and why. |

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

## Recorded Decisions

- Base branch: `codegym-v2`.
- Epic branch strategy: #184 is the contract base branch for the agent-built environment epic. Track A and Track B may use stacked branches from this contract branch while implementation is in progress.
- Integration path: completed Track A and Track B work will merge back into the contract branch before an integration branch is created. After integration validates the combined workflow, the completed epic work will merge back to `codegym-v2`.
- AgentRuntime incorporation: decided in #185. The `AgentRuntime` seam, ceilings, termination, cleanup, and telemetry from #172/#173 are adopted as the Track A base. The manifest concept is revised to produce `environment.manifest.v1`, and setup permissions and limits are defined at the runtime level. Runtime adapter selection remains open; both adapters are kept behind the interface.
- Runtime choice remains open: opencode, the purpose-built loop, or both may be used behind the builder boundary. Any runtime selected later must produce a builder result, proposed manifest, diagnostics, and telemetry without claiming readiness.

## Open Decisions

- What setup permissions are allowed during cold environment creation?
- What production limits apply to cold builds?
- Should Spring Boot fail fast when dependency-cache prerequisites are missing?
- Which exact second technology should be used for the first generic workflow demo?
- How should existing Python/Go saved exercises be migrated or retired?
