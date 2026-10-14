package practice

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/execution"
)

// Criterion 2: Run/Submit use the artifact's declared contract with
// backend-selected commands, and return the appropriate category.
// Criterion 4: faults and timeouts stay distinct from learner failures.

func greetCommands(t *testing.T) (build, test string) {
	t.Helper()
	manifest := loadGreetManifest(t)
	return manifest.Commands.Build, manifest.Commands.Test
}

func TestSubmitRunsBuildThenTest(t *testing.T) {
	service, _, _, runner := practiceService(t)
	session := openGreet(t, service, "owner-1")
	if err := service.WriteFile(session.ID, "owner-1", "solution.py", "learner work"); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	result, err := service.Submit(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Category != CategoryPassed {
		t.Fatalf("category = %q, want passed", result.Category)
	}
	build, test := greetCommands(t)
	specs := runner.recorded()
	if len(specs) != 2 || specs[0].Command != build || specs[1].Command != test {
		t.Fatalf("commands = %q, want exactly build then test", runner.commands())
	}
	if specs[0].ExecutionID == "" || specs[0].ExecutionID != specs[1].ExecutionID {
		t.Fatalf("execution IDs = %q/%q, want one shared isolated workspace",
			specs[0].ExecutionID, specs[1].ExecutionID)
	}
	for _, spec := range specs {
		if strings.Contains(spec.Command, "--version") {
			t.Fatalf("setup command ran on the practice path: %q", spec.Command)
		}
		if spec.SnapshotRef != "snap_greet_1" {
			t.Fatalf("snapshot = %q, want the sealed snapshot", spec.SnapshotRef)
		}
		if spec.NetworkPolicy != "block-all" {
			t.Fatalf("network policy = %q, want the translated block-all policy to reach the provider", spec.NetworkPolicy)
		}
		if spec.Limits != (execution.Limits{TimeoutSeconds: 60, MemoryMB: 256, NetworkMode: "block-all"}) {
			t.Fatalf("limits = %+v, want the existing execution limits contract delivered", spec.Limits)
		}
		if spec.MaxOutputBytes != 64*1024 || spec.MaxFileBytes != 256*1024 {
			t.Fatalf("caps = %d/%d, want the backend-owned output policy delivered",
				spec.MaxOutputBytes, spec.MaxFileBytes)
		}
		if len(spec.Files) != 1 || spec.Files[0].Path != "solution.py" || spec.Files[0].Content != "learner work" {
			t.Fatalf("files = %+v, want exactly the isolated overlay in execution.File form", spec.Files)
		}
		if spec.WorkDir != "." {
			t.Fatalf("workdir = %q, want the manifest workspace root", spec.WorkDir)
		}
	}
}

func TestRunUsesTestCommandFallback(t *testing.T) {
	service, _, _, runner := practiceService(t)
	session := openGreet(t, service, "owner-1")
	result, err := service.Run(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Category != CategoryPassed {
		t.Fatalf("category = %q, want passed", result.Category)
	}
	_, test := greetCommands(t)
	if got := runner.commands(); len(got) != 1 || got[0] != test {
		t.Fatalf("commands = %q, want exactly the test smoke check", got)
	}
	if len(result.Steps) != 1 || result.Steps[0].Stage != "test" {
		t.Fatalf("steps = %+v, want the single test stage", result.Steps)
	}
}

func TestRunUsesDeclaredRunCommand(t *testing.T) {
	service, source, _, runner := practiceService(t)
	artifact := sampleArtifact(t)
	artifact.Manifest.Commands.Run = "python3 solution.py"
	source.artifact = artifact
	runner.results["python3 solution.py"] = RunOutcome{ExitCode: 0, Output: "Hello, Ada!\n"}
	session := openGreet(t, service, "owner-1")
	result, err := service.Run(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Category != CategoryPassed {
		t.Fatalf("category = %q, want passed", result.Category)
	}
	if got := runner.commands(); len(got) != 1 || got[0] != "python3 solution.py" {
		t.Fatalf("commands = %q, want exactly the declared run command", got)
	}
	if len(result.Steps) != 1 || result.Steps[0].Stage != "run" {
		t.Fatalf("steps = %+v, want the single run stage", result.Steps)
	}
	if result.Output != "Hello, Ada!\n" {
		t.Fatalf("output = %q, want the learner program output preserved", result.Output)
	}
}

// Build failures carry the category and fixed summary with stage and
// exit: no build output lines are forwarded, because a filename mention
// cannot authorize arbitrary trailing text.

func TestBuildErrorStopsBeforeTest(t *testing.T) {
	service, _, _, runner := practiceService(t)
	build, _ := greetCommands(t)
	runner.results[build] = RunOutcome{ExitCode: 1, Output: "File \"solution.py\", line 2\n    retrun greet(name)\nSyntaxError: invalid syntax"}
	session := openGreet(t, service, "owner-1")
	result, err := service.Submit(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Category != CategoryBuildError {
		t.Fatalf("category = %q, want build-error", result.Category)
	}
	if result.Summary != "The build command failed." {
		t.Fatalf("summary = %q, want the safe fixed summary", result.Summary)
	}
	if len(runner.commands()) != 1 {
		t.Fatalf("commands = %q, want the test to never run after a build error", runner.commands())
	}
	if result.Output != "" {
		t.Fatalf("output = %q, want program output reserved for run steps", result.Output)
	}
	if len(result.Steps) != 1 || result.Steps[0].Stage != "build" || result.Steps[0].ExitCode != 1 {
		t.Fatalf("steps = %+v, want the single failed build stage", result.Steps)
	}
}

func TestTestFailureCategory(t *testing.T) {
	service, _, _, runner := practiceService(t)
	_, test := greetCommands(t)
	runner.results[test] = RunOutcome{ExitCode: 2, Output: "2 tests failed"}
	session := openGreet(t, service, "owner-1")
	result, err := service.Submit(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Category != CategoryTestFailure {
		t.Fatalf("category = %q, want test-failure", result.Category)
	}
	if len(result.Steps) != 2 || result.Steps[0].ExitCode != 0 || result.Steps[1].ExitCode != 2 {
		t.Fatalf("steps = %+v, want build pass then test exit 2", result.Steps)
	}
	if result.Output != "" {
		t.Fatalf("output = %q, want no test-step output retained", result.Output)
	}
}

// Test-step output is never projected: realistic tracebacks carrying
// hidden paths, assertion values, and protected source must leave no
// trace in the public result, while stage and exit stay visible.

func TestTestOutputNeverRetained(t *testing.T) {
	service, _, _, runner := practiceService(t)
	_, test := greetCommands(t)
	runner.results[test] = RunOutcome{ExitCode: 1, Output: `Traceback (most recent call last):
  File "protected/test_greet.py", line 12, in test_formal
    assert greet("Ada") == "Hello, Ada!"
  File "protected/test_greet.py", line 4, in helper
    return "Hello, " + name + "!"
AssertionError: expected 'Hello, Ada!' but got 'Hi Ada'
`}
	session := openGreet(t, service, "owner-1")
	result, err := service.Submit(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Category != CategoryTestFailure {
		t.Fatalf("category = %q, want test-failure", result.Category)
	}
	public := result.Summary + "\x00" + result.Output + "\x00" + string(result.Category)
	for _, stage := range result.Steps {
		public += "\x00" + stage.Stage
	}
	for _, secret := range []string{
		"protected/test_greet.py", // hidden path
		"test_greet",              // hidden name
		"test_formal",             // hidden assertion name
		"Hello, Ada!",             // expected answer value
		"Hi Ada",                  // actual value from hidden run
		"return \"Hello, \"",      // protected source content
	} {
		if strings.Contains(public, secret) {
			t.Fatalf("public result leaks %q: %+v", secret, result)
		}
	}
	if result.Output != "" {
		t.Fatalf("output = %q, want test-step output dropped", result.Output)
	}
}

func TestTimeoutAndFaultStayDistinct(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		service, _, _, runner := practiceService(t)
		_, test := greetCommands(t)
		runner.results[test] = RunOutcome{ExitCode: 0, Output: "slow", TimedOut: true}
		session := openGreet(t, service, "owner-1")
		result, err := service.Submit(context.Background(), session.ID, "owner-1")
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if result.Category != CategoryTimeout {
			t.Fatalf("category = %q, want timeout", result.Category)
		}
		if result.Output != "" {
			t.Fatalf("output = %q, want nothing retained from a timed-out step", result.Output)
		}
	})
	t.Run("command deadline", func(t *testing.T) {
		service, _, _, runner := practiceService(t)
		service.CommandTimeout = 20 * time.Millisecond
		session := openGreet(t, service, "owner-1")
		// Live parent context; the provider blocks until the per-command
		// context expires, then reports nil-error success. That success
		// must still produce timeout.
		runner.onRun = func(ctx context.Context, _ RunSpec) {
			<-ctx.Done()
		}
		result, err := service.Submit(context.Background(), session.ID, "owner-1")
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if result.Category != CategoryTimeout {
			t.Fatalf("category = %q, want timeout for success after the command deadline", result.Category)
		}
		if len(runner.commands()) != 1 {
			t.Fatalf("commands = %d, want exactly the expired build step", len(runner.commands()))
		}
	})
	t.Run("fault", func(t *testing.T) {
		service, _, _, runner := practiceService(t)
		_, test := greetCommands(t)
		runner.errs[test] = errors.New("sandbox unavailable")
		session := openGreet(t, service, "owner-1")
		result, err := service.Submit(context.Background(), session.ID, "owner-1")
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if result.Category != CategoryPlatformFault {
			t.Fatalf("category = %q, want platform-fault", result.Category)
		}
		if strings.Contains(result.Summary+result.Output, "sandbox unavailable") {
			t.Fatalf("provider diagnostic leaked: %+v", result)
		}
	})
}

// Tampering with backend state between open and execution cannot promote
// bad state: a broken manifest reports malformed-manifest and a foreign
// overlay file reports validation-failed, without invoking the provider.
// Direct state access below simulates backend-state tampering; learner
// APIs cannot reach it.

func TestUseTimeValidationCategories(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		service, _, _, runner := practiceService(t)
		session := openGreet(t, service, "owner-1")
		service.sessions[session.ID].manifest.Workspace = nil
		result, err := service.Run(context.Background(), session.ID, "owner-1")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if result.Category != CategoryMalformedManifest {
			t.Fatalf("category = %q, want malformed-manifest", result.Category)
		}
		if len(runner.commands()) != 0 {
			t.Fatalf("provider ran for a malformed manifest")
		}
	})
	t.Run("validation", func(t *testing.T) {
		service, _, _, runner := practiceService(t)
		session := openGreet(t, service, "owner-1")
		service.sessions[session.ID].files["evil.py"] = "intruder"
		result, err := service.Submit(context.Background(), session.ID, "owner-1")
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if result.Category != CategoryValidationFailed {
			t.Fatalf("category = %q, want validation-failed", result.Category)
		}
		if len(runner.commands()) != 0 {
			t.Fatalf("provider ran for an invalid overlay")
		}
	})
}

// Reads split by actor: the trusted harness observes the step
// visibility, while learner-controlled code observes the learner view in
// every scope — trusted manifest commands can execute learner code, so
// stage names alone never establish trust. Learner reads of hidden,
// reference, and verifier-owned files fail under build specs too, while
// harness reads there retain trusted assessment access.

func TestActorSeparatedReads(t *testing.T) {
	service, source, _, runner := practiceService(t)
	artifact := sampleArtifact(t)
	artifact.Manifest.Commands.Run = "python3 solution.py"
	source.artifact = artifact
	session := openGreet(t, service, "owner-1")
	if _, err := service.Run(context.Background(), session.ID, "owner-1"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := service.Submit(context.Background(), session.ID, "owner-1"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	specs := runner.recorded()
	if len(specs) != 3 {
		t.Fatalf("specs = %d, want run then build+test", len(specs))
	}
	withheld := []string{
		"protected/test_greet.py",
		"solutions/correct.py",
		"solutions/wrong.py",
		".codegym/judge.key",
	}
	for _, spec := range specs {
		for _, path := range withheld {
			if _, err := runner.observe(spec, ActorLearner, path); err == nil {
				t.Fatalf("learner-actor read of %q under %q spec succeeded, want FileNotFoundError", path, spec.Stage)
			} else if !strings.Contains(err.Error(), "FileNotFoundError") {
				t.Fatalf("learner-actor read err = %q, want gated open", err.Error())
			}
		}
	}
	if content, err := runner.observe(specs[0], ActorLearner, "solution.py"); err != nil {
		t.Fatalf("learner-scope overlay read: %v", err)
	} else if !strings.Contains(content, "greet") {
		t.Fatalf("learner overlay unreadable in learner scope")
	}
	if content, err := runner.observe(specs[1], ActorHarness, "protected/test_greet.py"); err != nil {
		t.Fatalf("harness trusted read under build: %v", err)
	} else if !strings.Contains(content, "SAMPLE_ASSERT") {
		t.Fatalf("harness lost trusted access to the assessment file")
	}
	if content, err := runner.observe(specs[1], ActorHarness, "solutions/correct.py"); err != nil {
		t.Fatalf("harness trusted read of reference material: %v", err)
	} else if !strings.Contains(content, "Hello, ") {
		t.Fatalf("harness reference read lost content")
	}
	if _, err := runner.observe(specs[0], ActorHarness, "protected/test_greet.py"); err == nil {
		t.Fatalf("harness read under learner scope succeeded, want the declared scope honored")
	} else if !strings.Contains(err.Error(), "FileNotFoundError") {
		t.Fatalf("harness read err = %q, want gated open", err.Error())
	}
}

// Partial and encoded extraction attempts fail identically at open:
// gating happens before any byte flows, so read length and encoding
// change nothing. The attempts below run their exfiltration code through
// Run; the fake refuses the open behaviorally from the overlay and
// scope — no preloaded blocking transcript — and the public result
// carries no hidden content.

func TestPartialAndEncodedExtractionBlocked(t *testing.T) {
	attempts := []struct {
		name string
		code string
	}{
		{"partial", "print(open('protected/test_greet.py').read(16))"},
		{"encoded", "print(__import__('base64').b64encode(open('protected/test_greet.py','rb').read()))"},
		{"reference", "print(open('solutions/correct.py').read())"},
	}
	for _, attempt := range attempts {
		t.Run(attempt.name, func(t *testing.T) {
			service, source, _, runner := practiceService(t)
			artifact := sampleArtifact(t)
			artifact.Manifest.Commands.Run = "python3 solution.py"
			source.artifact = artifact
			session := openGreet(t, service, "owner-1")
			if err := service.WriteFile(session.ID, "owner-1", "solution.py", attempt.code); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			result, err := service.Run(context.Background(), session.ID, "owner-1")
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if result.Category != CategoryTestFailure {
				t.Fatalf("category = %q, want test-failure for the crashed learner program", result.Category)
			}
			specs := runner.recorded()
			if len(specs) != 1 || specs[0].Visibility != FileScopeLearner {
				t.Fatalf("run executed outside learner scope: %+v", specs)
			}
			// The read itself was refused at open, in every form.
			if _, err := runner.observe(specs[0], ActorLearner, "protected/test_greet.py"); err == nil {
				t.Fatalf("%s read succeeded under learner scope", attempt.name)
			}
			public := result.Summary + "\x00" + result.Output + "\x00" + string(result.Category)
			for _, secret := range []string{
				"protected/test_greet",
				"solutions/correct",
				"SAMPLE_ASSERT",
				"Hello, Alice!",
			} {
				if strings.Contains(public, secret) {
					t.Fatalf("%s attempt leaks %q: %+v", attempt.name, secret, result)
				}
			}
			if !strings.Contains(result.Output, "FileNotFoundError") {
				t.Fatalf("output = %q, want the safe learner diagnostic preserved", result.Output)
			}
		})
	}
}

// Nil contexts normalize at Run/Submit entry instead of panicking
// downstream: context.WithoutCancel must never see nil, including on the
// record-storage path.

func TestRunSubmitNilContextWithRecords(t *testing.T) {
	service, _, _, _ := practiceService(t)
	service.Records = NewInMemoryRecordStore()
	session := openGreet(t, service, "owner-1")
	run, err := service.Run(nil, session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Run(nil): %v", err)
	}
	if run.Category != CategoryPassed || run.RecordID == "" {
		t.Fatalf("run = %+v, want passed with a stored record", run)
	}
	submitted, err := service.Submit(nil, session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Submit(nil): %v", err)
	}
	if submitted.Category != CategoryPassed || submitted.RecordID == "" {
		t.Fatalf("submit = %+v, want passed with a stored record", submitted)
	}
	if _, err := service.Records.GetRecord(context.Background(), run.RecordID); err != nil {
		t.Fatalf("GetRecord run: %v", err)
	}
	if _, err := service.Records.GetRecord(context.Background(), submitted.RecordID); err != nil {
		t.Fatalf("GetRecord submit: %v", err)
	}
}

// Manifest network postures other than isolated have no approved
// translation: they refuse at open rather than riding provider defaults.

func TestUnknownNetworkPolicyRefused(t *testing.T) {
	service, source, _, _ := practiceService(t)
	bad := sampleArtifact(t)
	bad.Manifest.Environment.Network = "corp-lan"
	source.artifact = bad
	_, err := service.OpenSession(context.Background(), "owner-1", ArtifactRef{Technology: "python"})
	if err == nil || !strings.Contains(err.Error(), "unknown network policy") {
		t.Fatalf("err = %v, want explicit unknown-policy refusal", err)
	}
}

// Cross-owner execution is refused even with a valid session ID.

func TestCrossOwnerExecutionRefused(t *testing.T) {
	service, _, _, runner := practiceService(t)
	session := openGreet(t, service, "owner-1")
	if _, err := service.Run(context.Background(), session.ID, "intruder"); err == nil {
		t.Fatalf("Run cross-owner = nil, want refusal")
	}
	if _, err := service.Submit(context.Background(), session.ID, "intruder"); err == nil {
		t.Fatalf("Submit cross-owner = nil, want refusal")
	}
	if _, err := service.Run(context.Background(), "no-such-session", "owner-1"); err == nil {
		t.Fatalf("Run unknown session = nil, want refusal")
	}
	if len(runner.commands()) != 0 {
		t.Fatalf("provider ran for refused requests")
	}
}

// Submit build→test shares one isolated execution workspace with build
// outputs preserved: the stateful fake passes its test step only when it
// observes the build marker in the same workspace, and a second
// operation starts clean under its own ID.

func TestSubmitSharesOneExecutionWorkspace(t *testing.T) {
	service, _, _, runner := practiceService(t)
	runner.stateful = true
	session := openGreet(t, service, "owner-1")
	first, err := service.Submit(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if first.Category != CategoryPassed {
		t.Fatalf("category = %q, want passed with build outputs preserved", first.Category)
	}
	second, err := service.Submit(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if second.Category != CategoryPassed {
		t.Fatalf("second category = %q, want passed", second.Category)
	}
	specs := runner.recorded()
	if len(specs) != 4 {
		t.Fatalf("commands = %d, want two build+test operations", len(specs))
	}
	firstID, secondID := specs[0].ExecutionID, specs[2].ExecutionID
	if firstID == "" || firstID != specs[1].ExecutionID {
		t.Fatalf("first operation IDs = %q/%q, want one shared workspace", specs[0].ExecutionID, specs[1].ExecutionID)
	}
	if secondID == "" || secondID == firstID || secondID != specs[3].ExecutionID {
		t.Fatalf("second operation IDs = %q/%q, want a distinct fresh workspace", specs[2].ExecutionID, specs[3].ExecutionID)
	}
	if got := runner.execState[firstID]["build/output.txt"]; got != "ok" {
		t.Fatalf("first workspace lacks the preserved build output")
	}
	if got := runner.execState[secondID]["build/output.txt"]; got != "ok" {
		t.Fatalf("second workspace lacks its own build output")
	}
	if len(runner.execState) != 2 {
		t.Fatalf("workspaces = %d, want exactly one isolated map per operation", len(runner.execState))
	}
}

// Results carry artifact and environment identity for execution records,
// and map onto the existing execution status vocabulary. Artifact fields
// have no home on existing records — the documented #184 addition.

func TestResultIdentityAndStatusMapping(t *testing.T) {
	service, _, _, _ := practiceService(t)
	session := openGreet(t, service, "owner-1")
	result, err := service.Submit(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
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
			t.Fatalf("%s = %q, want %q", field.name, field.got, field.want)
		}
	}
	if result.SessionID != session.ID {
		t.Fatalf("session = %q, want the issuing session", result.SessionID)
	}
	for category, want := range map[Category]execution.Status{
		CategoryPassed:            execution.StatusPassed,
		CategoryBuildError:        execution.StatusFailed,
		CategoryTestFailure:       execution.StatusFailed,
		CategoryTimeout:           execution.StatusTimeout,
		CategoryPlatformFault:     execution.StatusError,
		CategoryMalformedManifest: execution.StatusError,
		CategoryValidationFailed:  execution.StatusError,
	} {
		if got := category.ExecutionStatus(); got != want {
			t.Fatalf("category %q maps to %q, want %q", category, got, want)
		}
	}
}

// Build failures carry the category and fixed summary with stage and
// exit: no build output lines are forwarded at all, because a filename
// mention cannot authorize arbitrary trailing text.

func TestProtectedCompileErrorProjected(t *testing.T) {
	service, _, _, runner := practiceService(t)
	build, _ := greetCommands(t)
	runner.results[build] = RunOutcome{ExitCode: 1, Output: `File "protected/test_greet.py", line 27
    if actual != expected:
                ^
SyntaxError: invalid syntax
`}
	session := openGreet(t, service, "owner-1")
	result, err := service.Submit(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Category != CategoryBuildError {
		t.Fatalf("category = %q, want build-error", result.Category)
	}
	if result.Summary != "The build command failed." {
		t.Fatalf("summary = %q, want the safe fixed summary", result.Summary)
	}
	public := result.Summary + "\x00" + result.Output + "\x00" + string(result.Category)
	for _, secret := range []string{
		"protected/test_greet.py",
		"test_greet",
		"if actual != expected:",
		"SyntaxError",
		"line 27",
	} {
		if strings.Contains(public, secret) {
			t.Fatalf("public result leaks %q: %+v", secret, result)
		}
	}
	if result.Output != "" {
		t.Fatalf("output = %q, want program output reserved for run steps", result.Output)
	}
	if len(result.Steps) != 1 || result.Steps[0].Stage != "build" || result.Steps[0].ExitCode != 1 {
		t.Fatalf("steps = %+v, want the single failed build stage", result.Steps)
	}
}

// A filename mention cannot authorize publishing arbitrary trailing
// text: an editable filename prefixed to encoded hidden content, and
// mixed learner/protected diagnostic lines, must leave no payload in the
// public result or the stored record. Per-line structured diagnostics
// await a provider contract (see doc.go); until then, category plus
// fixed summary is the whole build-error surface.

func TestBuildErrorPublishesNoRawOutput(t *testing.T) {
	payload := "aGVsbG8sIEFsaWNlIQ=="
	cases := []struct {
		name    string
		output  string
		secrets []string
	}{
		{"prefixed-encoded",
			"solution.py: " + payload + "\n",
			[]string{payload, "solution.py:"}},
		{"mixed",
			"File \"solution.py\", line 2\n    if actual != expected:\nFile \"protected/test_greet.py\", line 27\nSyntaxError: bad\n",
			[]string{"solution.py\", line", "if actual != expected:", "protected/test_greet", "SyntaxError"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _, runner := practiceService(t)
			service.Records = NewInMemoryRecordStore()
			build, _ := greetCommands(t)
			runner.results[build] = RunOutcome{ExitCode: 1, Output: tc.output}
			session := openGreet(t, service, "owner-1")
			result, err := service.Submit(context.Background(), session.ID, "owner-1")
			if err != nil {
				t.Fatalf("Submit: %v", err)
			}
			if result.Category != CategoryBuildError {
				t.Fatalf("category = %q, want build-error", result.Category)
			}
			if result.Summary != "The build command failed." {
				t.Fatalf("summary = %q, want the safe fixed summary", result.Summary)
			}
			public := result.Summary + "\x00" + result.Output + "\x00" + string(result.Category)
			for _, secret := range tc.secrets {
				if strings.Contains(public, secret) {
					t.Fatalf("%s leaks %q: %+v", tc.name, secret, result)
				}
			}
			if result.Output != "" {
				t.Fatalf("%s output = %q, want nothing forwarded", tc.name, result.Output)
			}
			if result.RecordID == "" {
				t.Fatalf("%s record unlinked", tc.name)
			}
			record, err := service.Records.GetRecord(context.Background(), result.RecordID)
			if err != nil {
				t.Fatalf("GetRecord: %v", err)
			}
			stored := record.Summary + "\x00" + record.Output + "\x00" + string(record.Category)
			for _, secret := range tc.secrets {
				if strings.Contains(stored, secret) {
					t.Fatalf("%s stored record leaks %q: %+v", tc.name, secret, record)
				}
			}
			if record.Output != "" {
				t.Fatalf("%s stored output = %q, want nothing forwarded", tc.name, record.Output)
			}
		})
	}
}

// Run output can exfiltrate protected source when learner code prints
// it. The full verbatim content is replaced exactly; the exit-based
// category is unaffected.

func TestReferenceSourceExfiltrationProjected(t *testing.T) {
	service, source, store, runner := practiceService(t)
	artifact := sampleArtifact(t)
	artifact.Manifest.Commands.Run = "python3 solution.py"
	source.artifact = artifact
	protected := store.files["snap_greet_1"]["protected/test_greet.py"]
	runner.results["python3 solution.py"] = RunOutcome{ExitCode: 0, Output: protected}
	session := openGreet(t, service, "owner-1")
	result, err := service.Run(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Category != CategoryPassed {
		t.Fatalf("category = %q, want passed: redaction must not rewrite the exit", result.Category)
	}
	for _, secret := range []string{
		"SAMPLE_ASSERT",
		"STRIP_WHITESPACE",
		"Hello, Alice!",
		"Learners cannot edit this file",
		"test_greet",
	} {
		if strings.Contains(result.Output, secret) {
			t.Fatalf("projected output leaks %q", secret)
		}
	}
	if !strings.Contains(result.Output, "[redacted]") {
		t.Fatalf("output = %q, want the redaction marker", result.Output)
	}
}

// Retained run output is byte-capped after scrubbing so a cut can never
// leave a partial secret.

func TestProjectedOutputCapped(t *testing.T) {
	service, source, _, runner := practiceService(t)
	service.MaxOutputBytes = 16
	artifact := sampleArtifact(t)
	artifact.Manifest.Commands.Run = "python3 solution.py"
	source.artifact = artifact
	runner.results["python3 solution.py"] = RunOutcome{ExitCode: 0, Output: "Hello, Ada!\n" + strings.Repeat("y", 100)}
	session := openGreet(t, service, "owner-1")
	result, err := service.Run(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Category != CategoryPassed {
		t.Fatalf("category = %q, want passed", result.Category)
	}
	if !result.OutputTruncated || !strings.HasSuffix(result.Output, "…[truncated]") {
		t.Fatalf("output = %q, want the byte cap with marker", result.Output)
	}
	if !strings.HasPrefix(result.Output, "Hello, Ada!\n") {
		t.Fatalf("output = %q, want learner output before the cap", result.Output)
	}
}

// Sandbox limits travel explicitly: configured service values reach the
// provider spec verbatim.

func TestCustomLimitsReachProvider(t *testing.T) {
	service, _, _, runner := practiceService(t)
	service.CommandTimeout = 5 * time.Second
	service.MaxOutputBytes = 100
	service.MaxFileBytes = 200
	service.MemoryMB = 512
	session := openGreet(t, service, "owner-1")
	if _, err := service.Submit(context.Background(), session.ID, "owner-1"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	for _, spec := range runner.recorded() {
		if spec.Limits != (execution.Limits{TimeoutSeconds: 5, MemoryMB: 512, NetworkMode: "block-all"}) {
			t.Fatalf("limits = %+v, want the existing execution limits contract", spec.Limits)
		}
		if spec.MaxOutputBytes != 100 || spec.MaxFileBytes != 200 {
			t.Fatalf("caps = %d/%d, want the configured output policy", spec.MaxOutputBytes, spec.MaxFileBytes)
		}
	}
}

// Execution records retain artifact ID/version and environment identity
// alongside the distinct practice category, following the existing store
// pattern. Unconfigured storage leaves results unlinked; store faults are
// best-effort and never fail the learner result.

func TestExecutionRecordsRetainIdentity(t *testing.T) {
	service, _, _, _ := practiceService(t)
	service.Records = NewInMemoryRecordStore()
	session := openGreet(t, service, "owner-1")
	result, err := service.Submit(context.Background(), session.ID, "owner-1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.RecordID == "" {
		t.Fatalf("record ID empty, want the result linked to storage")
	}
	record, err := service.Records.GetRecord(context.Background(), result.RecordID)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	for _, field := range []struct {
		name string
		got  string
		want string
	}{
		{"session", record.SessionID, session.ID},
		{"owner", record.Owner, "owner-1"},
		{"artifact", record.ArtifactID, "greet-basics-2026-10-01"},
		{"version", record.ArtifactVersion, "0.1.0"},
		{"technology", record.Technology, "python"},
		{"environment", record.EnvironmentBase, "python-3.13"},
		{"network", record.NetworkPolicy, "block-all"},
	} {
		if field.got != field.want {
			t.Fatalf("record %s = %q, want %q", field.name, field.got, field.want)
		}
	}
	if record.Category != CategoryPassed || record.Status != execution.StatusPassed {
		t.Fatalf("record category/status = %q/%q, want passed/passed", record.Category, record.Status)
	}
	if len(record.Steps) != 2 {
		t.Fatalf("record steps = %+v, want build then test", record.Steps)
	}
	if record.CreatedAt.IsZero() {
		t.Fatalf("record has no timestamp")
	}
}

func TestRecordStorageBestEffort(t *testing.T) {
	t.Run("unconfigured", func(t *testing.T) {
		service, _, _, _ := practiceService(t)
		session := openGreet(t, service, "owner-1")
		result, err := service.Submit(context.Background(), session.ID, "owner-1")
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if result.Category != CategoryPassed || result.RecordID != "" {
			t.Fatalf("result = %+v, want passed with no record link", result)
		}
	})
	t.Run("fault", func(t *testing.T) {
		service, _, _, _ := practiceService(t)
		service.Records = &errRecordStore{}
		session := openGreet(t, service, "owner-1")
		result, err := service.Submit(context.Background(), session.ID, "owner-1")
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if result.Category != CategoryPassed || result.RecordID != "" {
			t.Fatalf("result = %+v, want passed despite the store fault", result)
		}
	})
	t.Run("missing", func(t *testing.T) {
		service, _, _, _ := practiceService(t)
		service.Records = NewInMemoryRecordStore()
		if _, err := service.Records.GetRecord(context.Background(), "no-such-record"); err != ErrRecordNotFound {
			t.Fatalf("err = %v, want explicit not-found", err)
		}
	})
}

type errRecordStore struct{}

func (s *errRecordStore) SaveRecord(context.Context, Record) error {
	return errors.New("record store unavailable")
}

func (s *errRecordStore) GetRecord(context.Context, string) (Record, error) {
	return Record{}, errors.New("record store unavailable")
}
