package practice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/execution"
)

// This file holds test-only fakes: a scripted artifact source serving the
// checked-in #188 greet sample, an in-memory content store, and a
// scriptable command runner. Fakes prove orchestration, gating,
// redaction, and that limits/policy reach the provider call — never real
// sandbox, network, or subprocess enforcement.

func loadGreetManifest(t *testing.T) environmentverify.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "environmentverify", "testdata", "greet", "manifest.json"))
	if err != nil {
		t.Fatalf("read greet manifest: %v", err)
	}
	manifest, err := environmentverify.ValidateBytes(data)
	if err != nil {
		t.Fatalf("parse greet manifest: %v", err)
	}
	return manifest
}

func loadGreetSolution(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "environmentverify", "testdata", "greet", "solution.py"))
	if err != nil {
		t.Fatalf("read greet solution: %v", err)
	}
	return string(data)
}

// loadProtectedTests reads the real hidden assessment file so leakage
// tests use realistic sensitive content, not invented secrets.
func loadProtectedTests(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "environmentverify", "testdata", "greet", "protected", "test_greet.py"))
	if err != nil {
		t.Fatalf("read protected tests: %v", err)
	}
	return string(data)
}

// loadSolutionsFile reads a real backend reference solution using the
// actual sample layout, so reference-material tests never rely on the
// fake's old synthetic paths.
func loadSolutionsFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "environmentverify", "testdata", "greet", "solutions", name))
	if err != nil {
		t.Fatalf("read solutions file: %v", err)
	}
	return string(data)
}

// sampleArtifact builds the trusted pair from the checked-in sample: a
// registry-passed record mirroring the manifest plus the manifest itself.
func sampleArtifact(t *testing.T) VerifiedArtifact {
	t.Helper()
	manifest := loadGreetManifest(t)
	return VerifiedArtifact{
		Record: environmentregistry.ArtifactRecord{
			ArtifactID:      manifest.Artifact.ID,
			ArtifactVersion: manifest.Artifact.Version,
			Technology:      manifest.Artifact.Technology,
			EnvironmentBase: manifest.Environment.Base,
			SetupCommand:    manifest.Commands.Setup,
			BuildCommand:    manifest.Commands.Build,
			TestCommand:     manifest.Commands.Test,
			SnapshotRef:     "snap_greet_1",
			Provenance: environmentregistry.Provenance{
				BuiltBy:       "test-loop",
				Owner:         "owner-1",
				BuilderRunID:  "buildrun_greet_1",
				VerifierRunID: "verify_greet_1",
			},
			Owner:           "owner-1",
			ValidationState: environmentregistry.ValidationPassed,
			Compatibility: environmentregistry.CompatibilityRules{
				Technology: manifest.Artifact.Technology,
				Objective:  manifest.Compatibility.RequestedObjective,
				ReuseTags:  append([]string(nil), manifest.Compatibility.ReuseTags...),
			},
		},
		Manifest: manifest,
	}
}

type artifactCall struct {
	owner string
	ref   ArtifactRef
}

type fakeArtifactSource struct {
	mu       sync.Mutex
	artifact VerifiedArtifact
	err      error
	calls    []artifactCall
}

func (f *fakeArtifactSource) LoadVerifiedArtifact(_ context.Context, owner string, ref ArtifactRef) (VerifiedArtifact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, artifactCall{owner: owner, ref: ref})
	if f.err != nil {
		return VerifiedArtifact{}, f.err
	}
	return f.artifact, nil
}

func (f *fakeArtifactSource) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type contentRead struct {
	snapshot string
	path     string
}

type fakeContentStore struct {
	mu    sync.Mutex
	files map[string]map[string]string
	err   error
	reads []contentRead
}

func (f *fakeContentStore) ReadArtifactFile(_ context.Context, snapshotRef, filePath string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, contentRead{snapshot: snapshotRef, path: filePath})
	if f.err != nil {
		return "", f.err
	}
	content, ok := f.files[snapshotRef][filePath]
	if !ok {
		return "", fmt.Errorf("fake content store has no file %q in snapshot %q", filePath, snapshotRef)
	}
	return content, nil
}

func (f *fakeContentStore) readPaths() []contentRead {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]contentRead(nil), f.reads...)
}

type fakeRunner struct {
	mu      sync.Mutex
	results map[string]RunOutcome
	errs    map[string]error
	specs   []RunSpec
	onRun   func(ctx context.Context, spec RunSpec)
	// stateful mode scopes one mutable file map per ExecutionID: the
	// build stage writes a marker the later stages must observe. Fresh
	// execution IDs start empty.
	stateful  bool
	execState map[string]map[string]string
	// base models the sealed artifact behind the snapshot using the actual
	// sample layout: learner and protected files plus backend
	// reference/verifier material no manifest lists (solutions/ review
	// files and the reserved dot-directory).
	base map[string]string
}

// observe models one file read under spec by an actor: the trusted
// harness observes the step visibility, while learner-controlled code
// observes the learner view in every scope — trusted manifest commands
// can execute learner-controlled code, so stage names alone never
// establish trust. Reads fail at open in every form, so full, partial,
// and encoded extraction fail identically with no byte flowing. Tests
// drive extraction attempts through this to pin the access rule the
// provider must implement.
func (f *fakeRunner) observe(spec RunSpec, actor Actor, filePath string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	overlay := map[string]string{}
	for _, file := range spec.Files {
		overlay[file.Path] = file.Content
	}
	sensitive := map[string]bool{}
	for _, name := range spec.Protected {
		sensitive[name] = true
	}
	visibility := spec.Visibility
	if actor == ActorLearner {
		visibility = FileScopeLearner
	}
	view := scopeFiles(visibility, overlay, f.base, sensitive)
	content, ok := view[filePath]
	if !ok {
		return "", fmt.Errorf("FileNotFoundError: [Errno 2] No such file or directory: %q", filePath)
	}
	return content, nil
}

// blockedReference models provider enforcement for learner-scope run
// steps: overlay code naming a protected path or reserved location is
// refused at open, before any byte flows. It returns the offending token
// — the full path for manifest entries, the full quoted literal for
// reserved locations — for a faithful blocked-open transcript. This
// models the conforming provider through the executed Run, not a
// preloaded transcript, so the test fails if the adapter ever sends
// learner code outside learner scope. Quoted-literal scanning is a fake
// approximation: production enforcement gates open() itself, whatever
// form the path takes.
func blockedReference(spec RunSpec) (token, line string, blocked bool) {
	if spec.Visibility != FileScopeLearner {
		return "", "", false
	}
	for _, file := range spec.Files {
		for _, text := range strings.Split(file.Content, "\n") {
			for _, candidate := range spec.Protected {
				if candidate != "" && strings.Contains(text, candidate) {
					return candidate, strings.TrimSpace(text), true
				}
			}
			for _, literal := range quotedLiterals(text) {
				if reservedLocation(literal) {
					return literal, strings.TrimSpace(text), true
				}
			}
		}
	}
	return "", "", false
}

// quotedLiterals extracts single- and double-quoted spans from one line,
// skipping backslash escapes. Fake-only approximation for recovering
// attempted paths; production gates the open call itself.
func quotedLiterals(line string) []string {
	var out []string
	for i := 0; i < len(line); i++ {
		if line[i] != '\'' && line[i] != '"' {
			continue
		}
		quote := line[i]
		j := i + 1
		for j < len(line) && line[j] != quote {
			if line[j] == '\\' {
				j++
			}
			j++
		}
		if j < len(line) {
			out = append(out, line[i+1:j])
		}
		i = j
	}
	return out
}

func (f *fakeRunner) RunCommand(ctx context.Context, spec RunSpec) (RunOutcome, error) {
	f.mu.Lock()
	record := spec
	record.Files = append([]execution.File(nil), spec.Files...)
	f.specs = append(f.specs, record)
	hook := f.onRun
	if f.stateful {
		if f.execState == nil {
			f.execState = map[string]map[string]string{}
		}
		state, ok := f.execState[spec.ExecutionID]
		if !ok {
			state = map[string]string{}
			f.execState[spec.ExecutionID] = state
		}
		f.mu.Unlock()
		if hook != nil {
			hook(ctx, spec)
		}
		if spec.Stage == environmentverify.StageBuild {
			f.mu.Lock()
			f.execState[spec.ExecutionID]["build/output.txt"] = "ok"
			f.mu.Unlock()
			return RunOutcome{ExitCode: 0}, nil
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.execState[spec.ExecutionID]["build/output.txt"] != "ok" {
			return RunOutcome{ExitCode: 1, Output: "missing build output"}, nil
		}
		return RunOutcome{ExitCode: 0}, nil
	}
	f.mu.Unlock()
	// Fire outside the lock: the hook may cancel the caller context to
	// simulate provider-side timing.
	if hook != nil {
		hook(ctx, spec)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	// Modeled enforcement for learner-scope run steps: overlay code
	// naming a protected path or reserved location is refused at open.
	// The blocked outcome derives from the executed attempt through
	// RunCommand — not from a preloaded transcript — so these tests fail
	// if the adapter ever sends learner code outside learner scope.
	if !f.stateful && spec.Stage == environmentverify.StageRun && spec.Visibility == FileScopeLearner {
		if token, line, blocked := blockedReference(spec); blocked {
			return RunOutcome{ExitCode: 1, Output: "Traceback (most recent call last):\n" +
				"  File \"solution.py\", line 1, in <module>\n" +
				"    " + line + "\n" +
				"FileNotFoundError: [Errno 2] No such file or directory: '" + token + "'\n"}, nil
		}
	}
	if err, ok := f.errs[spec.Command]; ok {
		return RunOutcome{}, err
	}
	if outcome, ok := f.results[spec.Command]; ok {
		return outcome, nil
	}
	return RunOutcome{ExitCode: 0}, nil
}

func (f *fakeRunner) recorded() []RunSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]RunSpec, len(f.specs))
	copy(out, f.specs)
	return out
}

func (f *fakeRunner) commands() []string {
	var out []string
	for _, spec := range f.recorded() {
		out = append(out, spec.Command)
	}
	return out
}

// practiceService wires a service whose source serves the sample, whose
// store holds the sample solution, and whose runner passes by default.
func practiceService(t *testing.T) (*Service, *fakeArtifactSource, *fakeContentStore, *fakeRunner) {
	t.Helper()
	source := &fakeArtifactSource{artifact: sampleArtifact(t)}
	store := &fakeContentStore{files: map[string]map[string]string{
		"snap_greet_1": {
			"solution.py":             loadGreetSolution(t),
			"protected/test_greet.py": loadProtectedTests(t),
		},
	}}
	runner := &fakeRunner{
		results: map[string]RunOutcome{},
		errs:    map[string]error{},
		base: map[string]string{
			"solution.py":              loadGreetSolution(t),
			"protected/test_greet.py":  loadProtectedTests(t),
			"solutions/correct.py":     loadSolutionsFile(t, "correct.py"),
			"solutions/wrong.py":       loadSolutionsFile(t, "wrong.py"),
			".codegym/judge.key":       "key-material",
			"lib/shared-helpers.index": "shared index",
		},
	}
	service := &Service{Artifacts: source, Contents: store, Runner: runner}
	return service, source, store, runner
}

// openGreet opens a session for the sample artifact. Test-only evidence.
func openGreet(t *testing.T, service *Service, owner string) Session {
	t.Helper()
	session, err := service.OpenSession(context.Background(), owner, ArtifactRef{Technology: "python"})
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	return session
}
