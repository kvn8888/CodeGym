// Package practice maps verified environment artifacts onto the practice
// execution contract (issue #190): starting a sample verified artifact,
// editing only allowlisted files, and running checks through existing
// backend Run/Submit and assessment paths while preserving sandbox limits
// and protected-file boundaries.
//
// Trust boundaries (deliberate):
//
//   - Learner requests carry a technology/version/objective/tag selector
//     and file contents only. They never supply readiness claims, trusted
//     manifests, or commands; every command executed comes from the
//     backend-loaded manifest.
//   - The trusted pair (registry record plus manifest) arrives through the
//     ArtifactSource boundary, never from the learner. OpenSession refuses
//     records that are not registry-passed and any record/manifest/request
//     version skew — versions are reported, never substituted.
//   - Registry Resolution carries records without manifests, so this
//     package does not reuse Resolve directly; the production source must
//     pair a resolved record with its manifest from the artifact store
//     (open integration item below).
//   - Sessions live server-side. Callers hold opaque session IDs plus a
//     display-only receipt; every file and execution operation re-resolves
//     the authoritative state for the requesting owner, so cross-owner
//     requests fail and public receipt mutation cannot change execution.
//     Hidden tests, reference solutions, and verifier-owned data never
//     enter a session: sessions hold an isolated copy of the editable set
//     only, and learner edits overlay that copy without touching the base.
//   - Access splits by actor, never by stage name alone: the trusted
//     command observes the step visibility, while learner-controlled code
//     paths observe the learner view in every scope — trusted manifest
//     commands can execute learner-controlled code, so build/test steps
//     confine learner reads exactly like run steps. Reads fail before any
//     byte flows, so full, partial, and encoded extraction fail
//     identically. The rule is executable contract (scopeFiles plus the
//     actor split), pinned by fakes that model it; production providers
//     must implement equivalent enforcement, which fakes cannot prove.
//     Reference/verifier material is excluded by location (reserved
//     solutions, reference, and dot-leading segments) regardless of
//     manifest listing, and tested against the actual sample layout.
//   - Results are a safe public projection, never published raw provider
//     output. They carry categories, fixed summaries, stage names, and
//     exit codes — never command strings and never build output lines.
//     A filename mention cannot authorize arbitrary trailing text (an
//     editable filename prefixed to encoded hidden content would
//     otherwise survive), so build failures publish only the category
//     and fixed summary; per-line build diagnostics await a structured
//     provider contract, which is the open #184 item below. Run steps
//     contribute capped program output; test steps contribute nothing.
//     Retained run output passes the provider result boundary — exact
//     replacement of protected/hidden names plus their full trusted
//     contents and every non-blank content line, then a byte cap.
//     Partial fragments in tool output remain a documented residual
//     limit.
//
// Runner reuse: execution goes through the local Runner interface, which
// mirrors environmentverify.CommandRunner (stage/command/workdir in,
// exit/output/timed-out out, returned error reserved for platform faults,
// context deadline for timeouts) and adds exactly what that contract
// cannot carry: a per-operation execution ID, the snapshot reference, the
// isolated file overlay in existing execution.File form (sorted by path),
// the translated block-all policy with the existing execution.Limits
// requirements (positive timeout/memory) plus explicit output/file caps,
// and per-stage visibility with actor split and the manifest sensitive
// list. No provider defaults are relied upon: unknown manifest postures
// refuse at open. Timeouts travel on the context; the command deadline is
// rechecked after the runner returns, so nil-error success past a
// per-command timeout still produces timeout. Contexts normalize at
// entry, so record storage never sees nil. Fake tests prove
// orchestration, gating, projection, scope declaration, and that
// limits/policy reach the provider call — never real sandbox, network,
// filesystem, or subprocess enforcement, which remain the provider's job.
//
// Execution workspaces: every step of one Run/Submit operation shares one
// provider execution workspace keyed by its ExecutionID, so build outputs
// persist for the test step. The provider owns that workspace —
// preserving state across the operation's steps and cleaning up
// afterwards; the adapter never reuses an ExecutionID. Sandbox limits,
// output policy, network policy, and cancellation travel explicitly per
// call so fakes can assert their delivery.
//
// Run versus Submit (explicit): Submit executes the manifest build command
// and then the test command, stopping at the first failure. Run executes
// the manifest's optional run command when declared; when absent, Run
// executes the test command as the documented smoke check. Setup
// (dependency installation) never runs on the practice path.
//
// Existing-contract connection (minimal, no modifications): the provider
// overlay reuses execution.File; practice categories map onto
// execution.Status via Category.ExecutionStatus for record-adjacent
// consumers. Modes already align by name with submission run/submit.
//
// Open integration items (explicit, not silently adopted):
//
//   - Production ArtifactSource (registry resolve plus manifest-store
//     pairing), ContentStore (base file reads), and Runner (sandboxed
//     boot of the snapshot, overlay of session files, isolated practice
//     network, teardown) are unwired; fakes stand in per #190.
//   - Existing execution Run and submission View records carry no
//     artifact identity, and submission TestResult needs parsed counts
//     this package deliberately does not fabricate — so no lossy
//     conversion is provided. The minimal proposed additions for #184
//     review are: artifact_id, artifact_version, environment base/version,
//     and network policy fields on execution/submission records; a
//     provider-side structured result contract for safe failure
//     drill-down and per-line build diagnostics (raw output of either
//     can never serve this); bounded
//     server-side session storage with TTL instead of the current
//     unbounded map; provider reserved-location policy for verifier-owned
//     files without manifest presence (reference solutions especially),
//     which the adapter cannot name from the manifest alone. This
//     package's own Result already carries identity, and no existing
//     contract is modified here.
//   - Provider-side reclamation of workspaces separated but never issued,
//     symlink-alias enforcement inside real sandboxes, and per-tenant
//     quota for practice runs are production items.
//   - Practice execution records persist in process memory; durable
//     history is a production integration limit like the other stores.
//   - No HTTP endpoints are added here; this service interface is the
//     stable backend surface for #191.
package practice
