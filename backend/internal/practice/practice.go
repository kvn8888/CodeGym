package practice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/execution"
)

// Backend-owned practice limits. Zero values on Service select these
// defaults; callers never supply limits. DefaultMemoryMB mirrors the
// seeded problem runtime default in the existing execution contract.
const (
	DefaultCommandTimeout = 60 * time.Second
	DefaultMaxOutputBytes = 64 * 1024
	DefaultMaxFileBytes   = 256 * 1024
	DefaultMemoryMB       = 256
)

// ArtifactRef selects a verified artifact. It carries an exact artifact
// identity pin plus a technology/version/objective/tag selector — never a
// manifest, command, or readiness claim. #191 callers must pin ArtifactID:
// technology/version alone can select another exercise and are never used
// for saved/resume resolution.
type ArtifactRef struct {
	// ArtifactID is the exact exercise pin. Empty accepts the served
	// artifact for the #190 technology selector; #191 browser flows must
	// set it and resume paths require it.
	ArtifactID string
	Technology string
	// Version is an exact pin. Empty accepts the served version, which is
	// always reported explicitly on the session and results.
	Version   string
	Objective string
	ReuseTags []string
}

// VerifiedArtifact is the trusted backend-loaded pair: a registry-passed
// record plus the manifest it was verified against. Only an
// ArtifactSource may produce one; learner input never does.
type VerifiedArtifact struct {
	Record   environmentregistry.ArtifactRecord
	Manifest environmentverify.Manifest
}

// ArtifactSource loads one trusted verified artifact for an owner. The
// #190 fake serves the checked-in sample; production pairs a registry
// resolution with its manifest from the artifact store.
type ArtifactSource interface {
	LoadVerifiedArtifact(ctx context.Context, owner string, ref ArtifactRef) (VerifiedArtifact, error)
}

// ContentStore reads base artifact file contents by snapshot reference.
// Reads back only what sessions copy at open; sessions never write here.
type ContentStore interface {
	ReadArtifactFile(ctx context.Context, snapshotRef, filePath string) (string, error)
}

// FileScope declares which artifact files the executed command may
// observe. Learner scope withholds protected, hidden, reference, and
// verifier-owned files from learner-executed code: reads fail before any
// byte flows, so full, partial, and encoded extraction fail identically.
// Full scope exposes the sealed artifact for trusted build/test commands,
// whose output stays internal and projected. The provider enforces the
// scope; fakes model it to pin the rule.
type FileScope string

// Execution visibility scopes.
const (
	// FileScopeLearner exposes the learner overlay plus non-sensitive
	// base files only. Used for run steps executing learner code.
	FileScopeLearner FileScope = "learner"
	// FileScopeFull exposes the sealed artifact for trusted commands.
	// Used for build/test steps.
	FileScopeFull FileScope = "full"
)

// RunSpec is one backend-executed practice command. Files reuses the
// existing execution file contract (sorted by path) for the isolated
// learner overlay; commands come from the trusted manifest, never the
// learner. Limits reuses the existing execution limits contract with the
// translated block-all policy; output and file caps travel alongside;
// deadlines travel on ctx. Visibility plus Protected tell the provider
// which files the trusted command may observe, while learner-controlled
// code paths stay confined to the learner view in every scope (see
// Runner). ExecutionID scopes every step of one Run/Submit operation to
// a single provider execution workspace. Fakes assert every field,
// proving delivery without proving provider enforcement.
type RunSpec struct {
	ExecutionID    string
	SnapshotRef    string
	Stage          string
	Command        string
	WorkDir        string
	Files          []execution.File
	NetworkPolicy  string
	Limits         execution.Limits
	MaxOutputBytes int
	MaxFileBytes   int
	Visibility     FileScope
	// Protected lists manifest-declared sensitive paths (protected plus
	// hidden tests), backend-derived. The provider withholds exactly
	// these from learner-scope execution alongside its own reserved
	// verifier-owned locations.
	Protected []string
}

// reservedLocation reports backend verifier-owned material by location,
// regardless of manifest listing: dot-leading segments (the existing
// reserved judge-protocol rule) and the solutions/reference segments
// that house reference solutions and review evidence. Learner views
// exclude these in every learner scope; the names are backend policy,
// proposed for #184 review, not learner input.
func reservedLocation(filePath string) bool {
	for _, segment := range strings.Split(filePath, "/") {
		if segment == "" || segment == "." || segment == ".." {
			continue
		}
		if strings.HasPrefix(segment, ".") {
			return true
		}
		if segment == "solutions" || segment == "reference" {
			return true
		}
	}
	return false
}

// scopeFiles composes the file view for one execution from the learner
// overlay, the artifact base, and the manifest-declared sensitive set.
// The overlay always wins on conflict. Full scope merges everything for
// the trusted command; learner scope drops sensitive paths and reserved
// verifier-owned locations, so learner-executed code cannot observe them
// in any read form. This pure rule is shared by fakes and required of
// providers.
func scopeFiles(scope FileScope, overlay, base map[string]string, sensitive map[string]bool) map[string]string {
	view := map[string]string{}
	if scope == FileScopeFull {
		for name, content := range base {
			view[name] = content
		}
	} else {
		for name, content := range base {
			if sensitive[name] || reservedLocation(name) {
				continue
			}
			view[name] = content
		}
	}
	for name, content := range overlay {
		view[name] = content
	}
	return view
}

// RunOutcome is the provider-observed outcome of one command. A non-zero
// exit is a normal check failure reported in the outcome; a returned
// error is a platform fault (except context.DeadlineExceeded, which is a
// timeout). The shape mirrors environmentverify.CommandRunner by design.
type RunOutcome struct {
	ExitCode int
	Output   string
	TimedOut bool
}

// Runner executes trusted practice commands, one call per stage command.
// All calls carrying one ExecutionID belong to a single isolated provider
// execution workspace: the provider must preserve filesystem state
// (build outputs) across that operation's steps and owns cleanup of the
// workspace afterwards. The adapter never reuses an ExecutionID.
//
// Access within an operation is split by actor, never by stage name
// alone: the trusted command observes spec.Visibility, while
// learner-controlled code paths (overlay files and anything they import
// or execute) observe the learner view in every scope — protected,
// hidden, reference, and verifier-owned files stay withheld from them
// even inside full-scope build/test steps, because trusted manifest
// commands can execute learner-controlled code. Outputs from steps that
// may execute learner code are projected, never published raw. The
// runner must honor context cancellation and terminate its work instead
// of returning success after the deadline.
type Runner interface {
	RunCommand(ctx context.Context, spec RunSpec) (RunOutcome, error)
}

// Actor names whose file reads a provider distinguishes: the trusted
// harness part of a command versus learner-controlled code paths.
type Actor string

// Execution actors.
const (
	// ActorHarness observes the step visibility: full scope for trusted
	// build/test commands, learner scope where the step declares it.
	ActorHarness Actor = "harness"
	// ActorLearner observes the learner view in every scope.
	ActorLearner Actor = "learner"
)

// Service is the #190 practice adapter. Sessions live server-side in the
// service: callers hold only opaque session IDs plus a display receipt,
// so public field mutation cannot change execution. All providers are
// injected; tests use fakes, production wires sandboxed implementations.
type Service struct {
	Artifacts ArtifactSource
	Contents  ContentStore
	Runner    Runner
	// Records persists terminal execution results. Nil skips persistence.
	Records RecordStore
	// Saved persists learner saved exercises for #191 resume. Nil disables
	// save/list/resume with an explicit error.
	Saved SavedExerciseStore
	// Simulated marks development/testing-only sample execution. When true,
	// results carry Simulated=true so clients label them explicitly. Never
	// enable in production.
	Simulated bool
	// CommandTimeout bounds each executed command. Zero selects default.
	CommandTimeout time.Duration
	// MaxOutputBytes caps decisive-step output kept on results.
	MaxOutputBytes int
	// MaxFileBytes caps one learner file write. Zero selects default.
	MaxFileBytes int
	// MemoryMB sizes the provider sandbox. Zero selects default; the
	// existing execution contract requires a positive value.
	MemoryMB int

	mu       sync.Mutex
	sessions map[string]*sessionState
}

// sessionState is the authoritative backend-owned session: identity,
// snapshot, trusted manifest, editable set, and isolated file overlay.
// Only the service touches it; file reads/writes go through the
// allowlisted operations below.
type sessionState struct {
	id              string
	owner           string
	workspaceID     string
	artifactID      string
	artifactVersion string
	technology      string
	environmentBase string
	networkPolicy   string
	snapshot        string
	// builderRunID and verifierRunID pin the exact backend build that
	// produced the snapshot. Resume compares them against the saved
	// reference so a same-ID/version replacement is reported explicitly
	// instead of silently restored. No hashes are used.
	builderRunID  string
	verifierRunID string
	editable      []string
	manifest      environmentverify.Manifest
	files         map[string]string
	// redact holds the exact backend-known sensitive tokens: protected
	// and hidden file names, basenames, and full contents. It never
	// leaves the backend; output projection replaces every occurrence.
	redact []string
}

func (s *Service) commandTimeout() time.Duration {
	if s.CommandTimeout > 0 {
		return s.CommandTimeout
	}
	return DefaultCommandTimeout
}

func (s *Service) maxOutputBytes() int {
	if s.MaxOutputBytes > 0 {
		return s.MaxOutputBytes
	}
	return DefaultMaxOutputBytes
}

func (s *Service) maxFileBytes() int {
	if s.MaxFileBytes > 0 {
		return s.MaxFileBytes
	}
	return DefaultMaxFileBytes
}

func (s *Service) memoryMB() int {
	if s.MemoryMB > 0 {
		return s.MemoryMB
	}
	return DefaultMemoryMB
}

// translateNetworkPolicy maps the manifest network posture onto the
// enforced execution vocabulary. Practice networking is always isolated:
// "none" translates explicitly to block-all; any other posture is
// refused rather than passed through on provider defaults.
func translateNetworkPolicy(manifest environmentverify.Manifest) (string, error) {
	if manifest.Environment == nil {
		return "", errors.New("practice artifact manifest has no environment")
	}
	// Practice runs never install dependencies: setup is complete by
	// promotion time, so setup-only carries the same practice invariant as
	// none, fully isolated practice networking.
	if network := strings.TrimSpace(manifest.Environment.Network); network == "none" || network == "setup-only" {
		return execution.NetworkModeBlockAll, nil
	}
	return "", fmt.Errorf("practice unknown network policy %q; practice networking is always isolated",
		manifest.Environment.Network)
}

// Session is the public display receipt for a practice session: identity
// fields safe for learners and #191. It carries no authority — every
// operation re-resolves the server-side state by ID and owner, so
// mutating a receipt changes nothing about execution.
type Session struct {
	ID              string
	Owner           string
	ArtifactID      string
	ArtifactVersion string
	Technology      string
	EnvironmentBase string
	NetworkPolicy   string
}

// OpenSession loads the trusted verified artifact for ref and opens an
// isolated server-side session seeded with a private copy of its editable
// files. Records that are not registry-passed, structurally invalid
// manifests, and any record/manifest/request version or identity skew are
// refused explicitly; versions are reported, never substituted.
func (s *Service) OpenSession(ctx context.Context, owner string, ref ArtifactRef) (Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Session{}, fmt.Errorf("practice open session: %w", err)
	}
	if strings.TrimSpace(owner) == "" {
		return Session{}, errors.New("practice open session requires an owner")
	}
	if strings.TrimSpace(ref.Technology) == "" {
		return Session{}, errors.New("practice open session requires a technology")
	}
	if s.Artifacts == nil {
		return Session{}, errors.New("practice artifact source is not configured")
	}
	if s.Contents == nil {
		return Session{}, errors.New("practice content store is not configured")
	}
	artifact, err := s.Artifacts.LoadVerifiedArtifact(ctx, owner, ref)
	if err != nil {
		return Session{}, fmt.Errorf("practice load artifact: %w", err)
	}
	if err := checkArtifact(artifact, ref); err != nil {
		return Session{}, err
	}
	manifest := cloneManifest(artifact.Manifest)
	networkPolicy, err := translateNetworkPolicy(manifest)
	if err != nil {
		return Session{}, err
	}
	editable := append([]string(nil), manifest.Workspace.LearnerEditable...)
	files := make(map[string]string, len(editable))
	for _, name := range editable {
		content, err := s.Contents.ReadArtifactFile(ctx, artifact.Record.SnapshotRef, name)
		if err != nil {
			return Session{}, fmt.Errorf("practice read base file %q: %w", name, err)
		}
		files[name] = content
	}
	// Capture the exact sensitive material for the output boundary:
	// protected and hidden file names plus their full trusted contents.
	// Reads here use the sealed snapshot, never learner input. A missing
	// sensitive file fails the open loudly — the boundary cannot be
	// established without it.
	redact, err := captureSensitive(ctx, s.Contents, artifact.Record.SnapshotRef, manifest)
	if err != nil {
		return Session{}, err
	}
	state := &sessionState{
		id:              newSessionID(),
		owner:           owner,
		artifactID:      artifact.Record.ArtifactID,
		artifactVersion: artifact.Record.ArtifactVersion,
		technology:      artifact.Record.Technology,
		environmentBase: artifact.Record.EnvironmentBase,
		networkPolicy:   networkPolicy,
		snapshot:        artifact.Record.SnapshotRef,
		builderRunID:    artifact.Record.Provenance.BuilderRunID,
		verifierRunID:   artifact.Record.Provenance.VerifierRunID,
		editable:        editable,
		manifest:        manifest,
		files:           files,
		redact:          redact,
	}
	s.mu.Lock()
	if s.sessions == nil {
		s.sessions = map[string]*sessionState{}
	}
	s.sessions[state.id] = state
	s.mu.Unlock()
	return state.receipt(), nil
}

// receipt projects the display-safe session fields.
func (state *sessionState) receipt() Session {
	return Session{
		ID:              state.id,
		Owner:           state.owner,
		ArtifactID:      state.artifactID,
		ArtifactVersion: state.artifactVersion,
		Technology:      state.technology,
		EnvironmentBase: state.environmentBase,
		NetworkPolicy:   state.networkPolicy,
	}
}

// require resolves one session for the requesting owner. Callers must not
// retain the pointer past the service mutex; snapshot instead.
func (s *Service) require(sessionID, owner string) (*sessionState, error) {
	state, ok := s.sessions[sessionID]
	if !ok || state == nil {
		return nil, errors.New("practice session not found")
	}
	if state.owner != owner {
		return nil, errors.New("practice session belongs to another owner")
	}
	return state, nil
}

// requireBrowser resolves one session for the requesting owner and
// workspace. Sessions opened without a workspace (the #190 path) accept
// any workspace for the same owner; browser sessions with a workspace pin
// require an exact match so cross-workspace access is rejected.
func (s *Service) requireBrowser(sessionID, owner, workspaceID string) (*sessionState, error) {
	state, ok := s.sessions[sessionID]
	if !ok || state == nil {
		return nil, errors.New("practice session not found")
	}
	if state.owner != owner {
		return nil, errors.New("practice session belongs to another owner")
	}
	if state.workspaceID != "" && state.workspaceID != workspaceID {
		return nil, errors.New("practice session belongs to another workspace")
	}
	return state, nil
}

// checkArtifact enforces the registry boundary: the record must be
// registry-passed, the manifest structurally valid, record and manifest
// mutually consistent on identity, and any requested technology/version
// pin exactly satisfied. An ArtifactID pin, when set, must match exactly:
// technology/version alone never select across exercises on pinned paths.
func checkArtifact(artifact VerifiedArtifact, ref ArtifactRef) error {
	record := artifact.Record
	if strings.TrimSpace(record.ArtifactID) == "" {
		return errors.New("practice artifact has no identity")
	}
	if pin := strings.TrimSpace(ref.ArtifactID); pin != "" && pin != record.ArtifactID {
		return fmt.Errorf("practice requested artifact %q but verified artifact is %q; artifacts are never substituted",
			pin, record.ArtifactID)
	}
	if record.ValidationState != environmentregistry.ValidationPassed {
		return fmt.Errorf("practice artifact %q version %q is not verified for practice",
			record.ArtifactID, record.ArtifactVersion)
	}
	if err := environmentverify.Validate(artifact.Manifest); err != nil {
		return fmt.Errorf("practice artifact manifest is malformed: %v", err)
	}
	manifest := artifact.Manifest
	if strings.TrimSpace(manifest.Artifact.ID) != record.ArtifactID {
		return fmt.Errorf("practice artifact/manifest identity mismatch for %q", record.ArtifactID)
	}
	if strings.TrimSpace(manifest.Artifact.Version) != record.ArtifactVersion {
		return fmt.Errorf("practice artifact/manifest version mismatch for %q", record.ArtifactID)
	}
	if strings.TrimSpace(manifest.Artifact.Technology) != record.Technology {
		return fmt.Errorf("practice artifact/manifest technology mismatch for %q", record.ArtifactID)
	}
	if strings.TrimSpace(manifest.Environment.Base) != record.EnvironmentBase {
		return fmt.Errorf("practice artifact/manifest environment mismatch for %q", record.ArtifactID)
	}
	if technology := strings.TrimSpace(ref.Technology); technology != record.Technology {
		return fmt.Errorf("practice technology %q does not match verified artifact %q", technology, record.Technology)
	}
	if pin := strings.TrimSpace(ref.Version); pin != "" && pin != record.ArtifactVersion {
		return fmt.Errorf("practice requested version %q but verified artifact is version %q; versions are never substituted",
			pin, record.ArtifactVersion)
	}
	return nil
}

// ListFiles returns the allowlisted editable set in stable order for the
// requesting owner. It is the only file enumeration a learner ever sees.
func (s *Service) ListFiles(sessionID, owner string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.require(sessionID, owner)
	if err != nil {
		return nil, err
	}
	names := append([]string(nil), state.editable...)
	sort.Strings(names)
	return names, nil
}

// ReadFile returns one overlay file for the requesting owner. The path
// must name a learner-editable file exactly after normalization;
// protected, hidden, absolute, escaping, and unknown paths are all
// refused alike, revealing nothing about which non-editable paths exist.
func (s *Service) ReadFile(sessionID, owner, filePath string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.require(sessionID, owner)
	if err != nil {
		return "", err
	}
	name, err := allowlistedPath(state.editable, filePath)
	if err != nil {
		return "", err
	}
	content, ok := state.files[name]
	if !ok {
		return "", fmt.Errorf("practice file %q is not available in this session", name)
	}
	return content, nil
}

// WriteFile applies one learner edit to the isolated overlay for the
// requesting owner. Only the editable set may be written, under the same
// normalization as reads; the trusted base artifact is never modified.
func (s *Service) WriteFile(sessionID, owner, filePath, content string) error {
	if len(content) > s.maxFileBytes() {
		return fmt.Errorf("practice file %q exceeds the %d byte limit", filePath, s.maxFileBytes())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.require(sessionID, owner)
	if err != nil {
		return err
	}
	name, err := allowlistedPath(state.editable, filePath)
	if err != nil {
		return err
	}
	state.files[name] = content
	return nil
}

// allowlistedPath normalizes a learner-supplied path and requires an
// exact match against the editable set. Lexical aliases resolving to the
// same file (a/../solution.py) are accepted as that file; absolute
// paths, dot segments that escape, and anything outside the set are
// refused with one uniform message.
func allowlistedPath(editable []string, filePath string) (string, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", errors.New("practice file path is required")
	}
	if path.IsAbs(filePath) {
		return "", fmt.Errorf("practice file %q is not learner-editable", filePath)
	}
	cleaned := path.Clean(filePath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("practice file %q is not learner-editable", filePath)
	}
	for _, name := range editable {
		if cleaned == name {
			return cleaned, nil
		}
	}
	return "", fmt.Errorf("practice file %q is not learner-editable", filePath)
}

// validateSessionFiles re-checks that an execution overlay holds exactly
// the editable set. The only writer is WriteFile, so failure indicates
// tampering or a defective writer — a validation failure, never silent
// acceptance.
func validateSessionFiles(editable []string, files map[string]string) error {
	if len(files) != len(editable) {
		return fmt.Errorf("practice workspace holds %d files, want the %d editable files",
			len(files), len(editable))
	}
	allowed := make(map[string]struct{}, len(editable))
	for _, name := range editable {
		allowed[name] = struct{}{}
	}
	for name := range files {
		if _, ok := allowed[name]; !ok {
			return fmt.Errorf("practice workspace holds non-editable file %q", name)
		}
	}
	return nil
}

// captureSensitive reads the exact sensitive material for the output
// boundary from the sealed snapshot: protected and hidden file names,
// their basenames, their full trusted contents, and every non-blank
// content line. Full contents stop verbatim exfiltration; per-line
// tokens stop echoed traceback source lines, which never contain a whole
// file. Filename-only lists cannot do either, and raw output
// pass-through is never allowed — every token here is replaced during
// projection. Empty tokens are dropped: replacing an empty string would
// corrupt all output. A learner line verbatim-equal to a protected line
// over-redacts by design (fail closed); fragments remain a documented
// residual limit.
func captureSensitive(ctx context.Context, store ContentStore, snapshotRef string, manifest environmentverify.Manifest) ([]string, error) {
	raws := protectedPaths(manifest)
	var tokens []string
	seen := map[string]struct{}{}
	add := func(token string) {
		token = strings.TrimSpace(token)
		if token == "" {
			return
		}
		if _, ok := seen[token]; ok {
			return
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	read := map[string]struct{}{}
	for _, raw := range raws {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		add(name)
		add(path.Base(name))
		if _, ok := read[name]; ok {
			continue
		}
		read[name] = struct{}{}
		content, err := store.ReadArtifactFile(ctx, snapshotRef, name)
		if err != nil {
			return nil, fmt.Errorf("practice read sensitive file %q: %w", name, err)
		}
		add(content)
		for _, line := range strings.Split(content, "\n") {
			add(line)
		}
	}
	return tokens, nil
}

// cloneManifest captures an independent deep copy of a manifest: every
// section pointer is re-pointed and every string slice re-backed, with
// nil slices staying nil.
func cloneManifest(manifest environmentverify.Manifest) environmentverify.Manifest {
	out := manifest
	if manifest.Artifact != nil {
		artifact := *manifest.Artifact
		out.Artifact = &artifact
	}
	if manifest.Environment != nil {
		environment := *manifest.Environment
		environment.DependencyFiles = cloneStrings(manifest.Environment.DependencyFiles)
		environment.Lockfiles = cloneStrings(manifest.Environment.Lockfiles)
		out.Environment = &environment
	}
	if manifest.Workspace != nil {
		workspace := *manifest.Workspace
		workspace.LearnerEditable = cloneStrings(manifest.Workspace.LearnerEditable)
		workspace.Protected = cloneStrings(manifest.Workspace.Protected)
		out.Workspace = &workspace
	}
	if manifest.Commands != nil {
		commands := *manifest.Commands
		out.Commands = &commands
	}
	if manifest.Assessment != nil {
		assessment := *manifest.Assessment
		assessment.VisibleTests = cloneStrings(manifest.Assessment.VisibleTests)
		assessment.HiddenTests = cloneStrings(manifest.Assessment.HiddenTests)
		out.Assessment = &assessment
	}
	if manifest.Compatibility != nil {
		compatibility := *manifest.Compatibility
		compatibility.ReuseTags = cloneStrings(manifest.Compatibility.ReuseTags)
		out.Compatibility = &compatibility
	}
	return out
}

// cloneStrings re-backs a string slice, preserving the nil versus
// explicitly-empty distinction exactly.
func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func newSessionID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "practice_sess_fallback"
	}
	return "practice_sess_" + hex.EncodeToString(raw[:])
}

func newExecutionID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "practice_exec_fallback"
	}
	return "practice_exec_" + hex.EncodeToString(raw[:])
}
