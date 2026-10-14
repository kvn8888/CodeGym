// Package environmentverify owns Track B manifest validation.
//
// It implements the verifier milestones from docs/environment-contract.md:
// parse the builder-proposed environment manifest, check its structure
// (milestone 1), validate the declared files against a backend-provided
// artifact root (milestone 2), and orchestrate backend-controlled
// setup/build/test stages through an injectable runner with
// contract-shaped rejected results (milestone 3). Generated commands run
// only through the injected runner; tests use a fake, and no sandboxes are
// provisioned here. Every command stage runs under a backend-owned deadline
// (see StageTimeouts); runners must honor cancellation and terminate their
// work. Fake tests prove deadline propagation at the orchestration layer,
// not real subprocess termination, which is the runner's job.
//
// A successful manifest validation does NOT establish readiness and does NOT
// allow promotion. Passing setup/build/test does not verify the environment
// either: with assessment inputs the verifier additionally requires the
// reviewed correct solution to pass and the wrong solution to fail for the
// intended reason before issuing a verified verdict, and promotion stays
// disallowed pending registry integration. The verifier result is the only
// object that can establish readiness. Builder prose, exit claims, and
// telemetry never influence verdicts.
//
// Open contract ambiguities (flagged, not silently decided):
//
//   - The contract names required top-level manifest fields but does not
//     specify subfield cardinality. The remaining nonempty rules
//     (workspace.learnerEditable, workspace.protected,
//     assessment.hiddenTests) are inferred from the valid example and the
//     promotion rules and stay pending shared decisions. Visible tests were
//     deliberately relaxed: an explicit empty set is accepted, so a
//     hidden-only assessment is not rejected by structural validation.
//   - The contract does not say whether dependency files and lockfiles may
//     both be empty. Both are accepted as explicit empty arrays so that
//     exercises without external dependencies are not rejected.
//   - The contract leaves reuse tags to later registry validation without
//     stating whether they may be empty. Presence is required; an explicit
//     empty set is accepted.
//   - The contract defines no enums for network mode, assessment type, or
//     technology. Milestone 1 accepts any non-blank value.
//   - The editable/protected overlap rule appears under promotion rules; it
//     is enforced here as structural validation and later stages may
//     re-check it against the real filesystem.
//   - Filesystem identity for overlap and hidden-test rules uses
//     device-plus-inode where the platform exposes it, so hard-link aliases
//     cannot bypass separation; platforms without inode identity fall back
//     to resolved paths.
//   - Repairability is a Track B policy pending shared review: manifest,
//     validation, setup, build, and test failures are repairable;
//     timeouts, platform faults, and the pending assessment gate are not.
//   - A verified verdict requires every gate: manifest, artifact,
//     setup/build/test, correct-solution pass, intended wrong-solution
//     failure, and review evidence. Verified never means promotable here:
//     promotion prerequisites belong to later registry integration.
//   - Stage deadline values are backend policy, not contract values: the
//     shared contract defines timeout as a failure stage but sets no
//     limits. DefaultStageTimeouts is a standing choice pending production
//     review.
//   - Assessment inputs (correct/wrong solutions, expected failure, review
//     evidence) are backend-controlled VerifyOptions, not manifest fields:
//     the manifest schema is unchanged. A shared assessment-input object
//     and harness outcome protocol are proposed for both-track review.
//   - Human sample review is assessment evidence for #188, not a
//     production rule established here: the checked-in sample reports
//     review pending with an explicit file-by-file checklist, unit tests
//     use synthetic-test-review labels that must never be mistaken for
//     approval, and the pending-review sample stays rejected. Whether
//     production assessment inputs need per-artifact human approval, and
//     what binds approval to content, are unresolved shared-interface
//     decisions for Track A coordination under #184 (see below).
//   - Intended wrong-solution failures require structured assertion
//     evidence: the observed record must equal the backend's expected code
//     and exact expected/actual values. Test id plus failure class alone
//     never suffices; missing or contradictory records are rejected.
//   - Assessment repairability is a Track B policy pending shared review:
//     outcome failures (correct fails, wrong passes or surprises, malformed
//     reports) are repairable; missing inputs and pending review are not.
//
// Integration boundaries (recorded, not implemented here — no registry,
// practice adapter, UI, Track A runtime, live demos, or migration work is
// built in this package):
//
//   - Verification is separate from registry promotion. A verified verdict
//     never sets promotion eligibility here (Promotion.Allowed is always
//     false in Track B). Issue #189 owns registry promotion and reuse
//     checks, including fresh boot, practice network-policy checks, and
//     compatibility evaluation, and must re-check promotion prerequisites
//     itself rather than trusting the verdict alone.
//   - Assessment inputs apply to the trusted base artifact, not to a
//     learner's edited solution: reference solution overlays in
//     AssessmentSpec validate the base exercise, while learner work is
//     assessed against the verified base at practice time.
//   - Sample solutions/ and protected fixture files are backend-only test
//     data. Future practice workspaces must expose only permitted files:
//     the learner-editable set from the verified manifest, never protected
//     or hidden assessment files. Practice, UI, and resume behavior belong
//     to #190-#191.
//   - Detailed verifier evidence (commands, outputs, assertion records)
//     stays internal to the backend. Future learner-facing responses must
//     receive safe failure categories derived from diagnostics, never raw
//     evidence or protected content.
//   - Artifact/version identity is preserved end to end: every
//     VerifierResult carries the builderRunId under verification, and
//     assessment inputs bind to the manifest's artifact id/version.
//     Downstream code associates a verdict with the artifact actually
//     verified by joining verifierRunId/builderRunId plus artifact
//     id/version — never by artifact id/version alone, which proves
//     nothing about content integrity on its own.
//   - AssessmentSpec and the TestOutcome/AssessmentReport harness protocol
//     remain proposed shared-interface additions for both-track review
//     under #184. The agent manifest schema is unchanged by them: they
//     travel beside VerifyOptions, and nothing here silently adopts them
//     into the shared contract. Open coordination items: the exact
//     assessment-input object shape, the harness outcome protocol,
//     production assessment-execution responsibility (sandbox, process
//     supervision, workspace lifecycle, and cleanup belong to the
//     production runner, which must honor cancellation and terminate its
//     work — fake runners used in tests prove orchestration deadlines,
//     not real subprocess enforcement), and what, if anything, binds
//     production approval to reviewed content.
//   - Live generic-Stack demos belong to #192 and existing-exercise
//     migration to #193; neither is claimed or constrained here.
package environmentverify
