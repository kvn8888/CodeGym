// Package environmentregistry owns Track B artifact promotion and reuse
// (issue #189): it populates a registry from backend-verified builds and
// serves compatible artifacts for practice without repeating dependency
// installation. This is not a manually curated technology list.
//
// Promotion authority (deliberate, flagged for both-track review under
// #184): the registry distinguishes #188's registry-pending state from an
// explicit backend denial. Registry-pending — verified verdict with
// promotion disallowed carrying #188's standing pending reason — means
// "not yet decided, registry prerequisites still to evaluate", and the
// registry proceeds to its own gates. Any other disallowed stance is an
// explicit backend denial, which registry gates cannot override to
// allowed=true. The receipt preserves the verifier result's promotion
// stance verbatim while deciding independently, so a future allowed=true
// is never trusted as authorization either. Pending-vs-denied recognition
// by reason-string quote is explicitly provisional: a stopgap that fails
// closed on rewording, with a machine-readable signal an open #184 item.
// The shared-contract example showing verifier-issued
// promotion.allowed=true therefore does not describe this flow;
// reconciling the example with registry-owned promotion is an open #184
// coordination item. The shared schema is unchanged by this package; any
// schema change needs coordinated review.
//
// Preparation association without content hashes: VerifyPreparation
// derives the preparation exclusively from the builder handoff being
// verified — never from a second, independently assembled manifest — and
// captures it as an independent deep copy (every section re-pointed,
// every slice re-backed with nil preserved, verdict evidence likewise
// copied), so later caller mutations cannot alter what was verified. The
// seal enforces evidence-command binding and promotion authority before
// sealing, with no boot or storage. Promote accepts only the sealed
// association and derives manifest, snapshot, and run from it alone: the
// request carries no caller-supplied preparation, so substitution is
// inexpressible rather than merely checked. Command equality constrains
// executed stages only and is not claimed as exact binding; byte-level
// content binding beyond the seal stays open with no prescribed
// mechanism.
//
// Open cross-track decisions (explicit, not silently adopted):
//
//   - The registry record shape, compatibility rule semantics (exact
//     technology/version/objective/tags matching), the Preparation and
//     VerifiedPreparation shapes, the VerifyPreparation/Promote boundary,
//     the BuildRequest/BuildOutput handoff shape (including the
//     exact-version pin and the opaque builder-owned BuilderResult),
//     resolution statuses (including output-mismatch and the narrowed
//     incompatible-version, which reports only a pin matching no promoted
//     artifact — a pin that exists without a compatible exercise is an
//     ordinary miss served by builder fallback), the receipt shape
//     and its reconciliation with the verifier promotion field, the
//     fail-closed quota/verifier-configured/owner requirements, workspace
//     lifetime rules, and cancellation/cleanup policy are Track B
//     proposals for #184 review, not shared-contract changes. The agent
//     manifest schema is unchanged.
//   - Same-ID/version replacement on save (re-promoting an artifact ID at
//     an existing version overwrites the record) is recorded here as a
//     compatibility decision for review, not redesigned in this pass:
//     fallback builds for an existing version converge the record onto
//     the latest verified preparation instead of forking versions.
//   - The builder-to-verifier handoff preserves the builder track's real
//     BuilderResult (run ID, runtime, proposed manifest path, diagnostics)
//     untouched; the registry fabricates no runtime metadata. Mapping that
//     handoff onto the production #188 verifier — resolving SnapshotRef to
//     a filesystem artifact root, confirming the recorded Manifest matches
//     the file at ProposedManifestPath, and supplying runner, verifier run
//     ID, and assessment inputs — is an open production-adapter item.
//   - Boot teardown runs on a cancellation-decoupled context bounded by
//     the backend-owned cleanup timeout (DefaultCleanupTimeout unless the
//     service sets CleanupTimeout), so a dead or deadline-less caller
//     context can neither skip provider cleanup nor leave it unbounded.
//     The registry owns boot lifetime on every path — success, check
//     failure, separation failure, cancellation, and cleanup timeout —
//     and never issues a workspace from a boot whose teardown failed.
//     Provider-side workspace garbage collection for workspaces separated
//     but never issued (e.g. dropped on late cancellation) is a
//     production lifetime item: the registry closes the boot it controls
//     and never issues such workspaces.
//   - Production assessment execution (sandbox, process supervision,
//     cleanup) stays with the production runner; fake providers used in
//     tests prove orchestration and gating, never production
//     sandbox/network enforcement.
//   - Durable persistence, real snapshot boot, live builder execution,
//     practice/UI/resume (#190-#191), live demos (#192), and migration
//     (#193) are out of scope here.
package environmentregistry
