package practice

import (
	"context"
	"strings"
	"testing"
)

// Criterion 1: starting the sample verified artifact exposes only the
// allowlisted editable files, seeded from the trusted base.

// Criterion 3 (read half): hidden tests and reference solutions cannot be
// viewed through practice paths.

func TestOpenSessionExposesOnlyEditableFiles(t *testing.T) {
	service, _, _, _ := practiceService(t)
	session := openGreet(t, service, "owner-1")
	files, err := service.ListFiles(session.ID, "owner-1")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "solution.py" {
		t.Fatalf("ListFiles = %q, want exactly the editable set", files)
	}
	content, err := service.ReadFile(session.ID, "owner-1", "solution.py")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(content, "greet") {
		t.Fatalf("editable content = %q, want the seeded base solution", content)
	}
	if session.ArtifactID != "greet-basics-2026-10-01" || session.ArtifactVersion != "0.1.0" {
		t.Fatalf("session identity = %s %s, want the served version reported",
			session.ArtifactID, session.ArtifactVersion)
	}
	if session.Owner != "owner-1" || session.ID == "" {
		t.Fatalf("session = %+v, want an identified owner session", session)
	}
}

func TestForbiddenReadsRefused(t *testing.T) {
	service, _, _, _ := practiceService(t)
	session := openGreet(t, service, "owner-1")
	for _, path := range []string{
		"protected/test_greet.py",              // hidden test, exact
		"protected/../protected/test_greet.py", // alias that still names it
		"./protected/test_greet.py",
		"/solution.py", // absolute
		"/protected/test_greet.py",
		"..",
		"../solution.py",
		"",
		"other.py",             // unknown
		".codegym/result.json", // reserved-looking
		"solution.py.bak",
	} {
		if _, err := service.ReadFile(session.ID, "owner-1", path); err == nil {
			t.Fatalf("ReadFile(%q) = nil, want refusal", path)
		} else if !strings.Contains(err.Error(), "learner-editable") && !strings.Contains(err.Error(), "required") {
			t.Fatalf("ReadFile(%q) err = %q, want the uniform boundary message", path, err.Error())
		}
	}
}

// Base reads stay inside manifest-declared paths: editable files for
// seeding plus protected/hidden files for the output boundary. Reference
// solutions appear in no manifest set, so no read can name them — they
// cannot leak even if a store held them.
func TestBaseReadsLimitedToManifestPaths(t *testing.T) {
	service, _, store, _ := practiceService(t)
	openGreet(t, service, "owner-1")
	allowed := map[string]bool{"solution.py": false, "protected/test_greet.py": false}
	for _, read := range store.readPaths() {
		if read.snapshot != "snap_greet_1" {
			t.Fatalf("base read = %+v, want the sealed snapshot only", read)
		}
		if _, ok := allowed[read.path]; !ok {
			t.Fatalf("base read = %+v, want only manifest-declared paths", read)
		}
		allowed[read.path] = true
	}
	for path, seen := range allowed {
		if !seen {
			t.Fatalf("manifest path %q never read", path)
		}
	}
}

// Criterion 3 (write half): only the editable set may be written, and
// edits land on the isolated overlay — the trusted base never changes.

func TestForbiddenWritesRefused(t *testing.T) {
	service, _, _, _ := practiceService(t)
	session := openGreet(t, service, "owner-1")
	before, err := service.ReadFile(session.ID, "owner-1", "solution.py")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, path := range []string{
		"protected/test_greet.py",
		"protected/../protected/test_greet.py",
		"/solution.py",
		"../solution.py",
		"",
		"other.py",
	} {
		if err := service.WriteFile(session.ID, "owner-1", path, "intruder"); err == nil {
			t.Fatalf("WriteFile(%q) = nil, want refusal", path)
		}
	}
	after, err := service.ReadFile(session.ID, "owner-1", "solution.py")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if after != before {
		t.Fatalf("overlay changed after refused writes")
	}
}

func TestEditsAreIsolatedPerSession(t *testing.T) {
	service, _, store, _ := practiceService(t)
	first := openGreet(t, service, "owner-1")
	if err := service.WriteFile(first.ID, "owner-1", "solution.py", "learner work"); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Alias to the same file resolves to the same entry.
	if err := service.WriteFile(first.ID, "owner-1", "sub/../solution.py", "aliased work"); err != nil {
		t.Fatalf("WriteFile alias: %v", err)
	}
	content, err := service.ReadFile(first.ID, "owner-1", "./solution.py")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if content != "aliased work" {
		t.Fatalf("overlay content = %q, want the latest learner edit", content)
	}
	// The base store is untouched and later sessions reseed clean.
	second := openGreet(t, service, "owner-1")
	fresh, err := service.ReadFile(second.ID, "owner-1", "solution.py")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(fresh, "learner work") || strings.Contains(fresh, "aliased work") {
		t.Fatalf("fresh session carries previous edits: %q", fresh)
	}
	for _, read := range store.readPaths() {
		if read.snapshot != "snap_greet_1" || (read.path != "solution.py" && read.path != "protected/test_greet.py") {
			t.Fatalf("unexpected base read %+v", read)
		}
	}
	if first.ID == second.ID {
		t.Fatalf("sessions share an ID")
	}
}

// Requesting-owner authorization: every file operation re-resolves the
// server-side session for the caller. Cross-owner requests fail even
// with a valid session ID, and unknown IDs fail closed.

func TestCrossOwnerFileAccessRefused(t *testing.T) {
	service, _, _, _ := practiceService(t)
	session := openGreet(t, service, "owner-1")
	if _, err := service.ListFiles(session.ID, "intruder"); err == nil {
		t.Fatalf("ListFiles cross-owner = nil, want refusal")
	}
	if _, err := service.ReadFile(session.ID, "intruder", "solution.py"); err == nil {
		t.Fatalf("ReadFile cross-owner = nil, want refusal")
	}
	if err := service.WriteFile(session.ID, "intruder", "solution.py", "x"); err == nil {
		t.Fatalf("WriteFile cross-owner = nil, want refusal")
	}
	if _, err := service.ListFiles("no-such-session", "owner-1"); err == nil {
		t.Fatalf("ListFiles unknown session = nil, want refusal")
	}
	// The rightful owner is unaffected.
	if _, err := service.ReadFile(session.ID, "owner-1", "solution.py"); err != nil {
		t.Fatalf("ReadFile rightful owner: %v", err)
	}
}

// Public receipt mutation cannot change execution: identity, snapshot,
// commands, and policy resolve server-side. Tampering with the receipt
// must leave the provider call and result identity untouched.

func TestReceiptTamperingChangesNothing(t *testing.T) {
	service, _, _, runner := practiceService(t)
	session := openGreet(t, service, "owner-1")
	session.Owner = "intruder"
	session.ArtifactID = "evil-id"
	session.ArtifactVersion = "9.9.9"
	session.Technology = "ruby"
	session.EnvironmentBase = "evil-base"
	session.NetworkPolicy = "open"
	result, err := service.Submit(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Category != CategoryPassed {
		t.Fatalf("category = %q, want passed", result.Category)
	}
	specs := runner.recorded()
	if len(specs) != 2 {
		t.Fatalf("commands = %d, want build then test", len(specs))
	}
	if specs[0].NetworkPolicy != "block-all" {
		t.Fatalf("provider policy = %q, want the sealed policy, not the tampered receipt", specs[0].NetworkPolicy)
	}
	for _, field := range []struct {
		name string
		got  string
		want string
	}{
		{"artifact", result.ArtifactID, "greet-basics-2026-10-01"},
		{"version", result.ArtifactVersion, "0.1.0"},
		{"technology", result.Technology, "python"},
		{"environment", result.EnvironmentBase, "python-3.13"},
		{"network", result.NetworkPolicy, "block-all"},
	} {
		if field.got != field.want {
			t.Fatalf("result %s = %q, want the sealed value %q", field.name, field.got, field.want)
		}
	}
}

// The visibility rule as executable contract: full scope merges base
// with overlay override; learner scope withholds manifest-sensitive
// paths and reserved verifier-owned locations — regardless of where in
// the tree they sit — while always showing the overlay. Providers must
// implement equivalent semantics.

func TestScopeFilesRule(t *testing.T) {
	base := map[string]string{
		"solution.py":             "base",
		"protected/test_greet.py": "hidden",
		".codegym/judge.key":      "key",
		"lib/util.py":             "util",
		"solutions/correct.py":    "reference",
		"nested/deep/reference/x": "nested-reference",
	}
	overlay := map[string]string{"solution.py": "learner"}
	sensitive := map[string]bool{"protected/test_greet.py": true}
	full := scopeFiles(FileScopeFull, overlay, base, sensitive)
	for name, want := range map[string]string{
		"solution.py":             "learner",
		"protected/test_greet.py": "hidden",
		".codegym/judge.key":      "key",
		"lib/util.py":             "util",
		"solutions/correct.py":    "reference",
		"nested/deep/reference/x": "nested-reference",
	} {
		if full[name] != want {
			t.Fatalf("full scope %q = %q, want %q", name, full[name], want)
		}
	}
	learner := scopeFiles(FileScopeLearner, overlay, base, sensitive)
	for name, want := range map[string]string{
		"solution.py": "learner",
		"lib/util.py": "util",
	} {
		if learner[name] != want {
			t.Fatalf("learner scope %q = %q, want %q", name, learner[name], want)
		}
	}
	for _, withheld := range []string{
		"protected/test_greet.py",
		".codegym/judge.key",
		"solutions/correct.py",
		"nested/deep/reference/x",
	} {
		if _, ok := learner[withheld]; ok {
			t.Fatalf("learner scope exposes %q", withheld)
		}
	}
	empty := scopeFiles(FileScopeLearner, map[string]string{}, base, sensitive)
	if _, ok := empty["protected/test_greet.py"]; ok {
		t.Fatalf("learner scope exposes protected files without an overlay")
	}
}

// The adapter declares visibility per stage: trusted build/test steps
// run full-scope with the manifest sensitive list; run steps executing
// learner code run learner-scope.

func TestStageVisibilityScopes(t *testing.T) {
	service, _, _, runner := practiceService(t)
	session := openGreet(t, service, "owner-1")
	if _, err := service.Submit(context.Background(), session.ID, "owner-1"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	specs := runner.recorded()
	if len(specs) != 2 {
		t.Fatalf("specs = %d, want build then test", len(specs))
	}
	for _, spec := range specs {
		if spec.Visibility != FileScopeFull {
			t.Fatalf("stage %q visibility = %q, want full for trusted commands", spec.Stage, spec.Visibility)
		}
		if len(spec.Protected) != 1 || spec.Protected[0] != "protected/test_greet.py" {
			t.Fatalf("protected = %q, want the manifest sensitive list", spec.Protected)
		}
	}
	service2, source2, _, runner2 := practiceService(t)
	artifact := sampleArtifact(t)
	artifact.Manifest.Commands.Run = "python3 solution.py"
	source2.artifact = artifact
	second := openGreet(t, service2, "owner-1")
	if _, err := service2.Run(context.Background(), second.ID, "owner-1"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	runSpecs := runner2.recorded()
	if len(runSpecs) != 1 {
		t.Fatalf("specs = %d, want exactly the run step", len(runSpecs))
	}
	if runSpecs[0].Visibility != FileScopeLearner {
		t.Fatalf("run visibility = %q, want learner for learner-executed code", runSpecs[0].Visibility)
	}
	if len(runSpecs[0].Protected) != 1 || runSpecs[0].Protected[0] != "protected/test_greet.py" {
		t.Fatalf("protected = %q, want the manifest sensitive list on every scope", runSpecs[0].Protected)
	}
}

// The registry boundary: unverified records and any version or identity
// skew are refused before a session exists. Learner requests carry no
// readiness to override this.

func TestOpenSessionRejectsUnverifiedArtifact(t *testing.T) {
	service, source, _, _ := practiceService(t)
	bad := sampleArtifact(t)
	bad.Record.ValidationState = ""
	source.artifact = bad
	_, err := service.OpenSession(context.Background(), "owner-1", ArtifactRef{Technology: "python"})
	if err == nil || !strings.Contains(err.Error(), "not verified") {
		t.Fatalf("err = %v, want an explicit unverified refusal", err)
	}
}

func TestOpenSessionRejectsVersionSkew(t *testing.T) {
	pinned := func() (*Service, *fakeArtifactSource) {
		service, source, _, _ := practiceService(t)
		return service, source
	}
	t.Run("pin", func(t *testing.T) {
		service, _ := pinned()
		_, err := service.OpenSession(context.Background(), "owner-1",
			ArtifactRef{Technology: "python", Version: "9.9.9"})
		if err == nil || !strings.Contains(err.Error(), "never substituted") {
			t.Fatalf("err = %v, want explicit pin refusal without substitution", err)
		}
	})
	t.Run("record-manifest", func(t *testing.T) {
		service, source := pinned()
		bad := sampleArtifact(t)
		bad.Manifest.Artifact.Version = "0.2.0"
		source.artifact = bad
		_, err := service.OpenSession(context.Background(), "owner-1", ArtifactRef{Technology: "python"})
		if err == nil || !strings.Contains(err.Error(), "mismatch") {
			t.Fatalf("err = %v, want explicit record/manifest mismatch", err)
		}
	})
	t.Run("technology", func(t *testing.T) {
		service, _ := pinned()
		_, err := service.OpenSession(context.Background(), "owner-1", ArtifactRef{Technology: "ruby"})
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("err = %v, want explicit technology mismatch", err)
		}
	})
}
