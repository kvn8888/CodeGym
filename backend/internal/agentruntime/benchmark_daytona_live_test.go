package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	apiclient "github.com/daytona/clients/api-client-go"
	"github.com/daytona/clients/sdk-go/pkg/daytona"
	"github.com/daytona/clients/sdk-go/pkg/options"
	"github.com/daytona/clients/sdk-go/pkg/types"
	"github.com/kvn8888/codegym/backend/internal/execution"
)

const (
	daytonaBenchmarkLabel    = "fix-agent-benchmark"
	daytonaBenchmarkSnapshot = execution.GoSnapshotName
)

type daytonaBenchmarkSuite struct {
	client *daytona.Client
	meter  *daytonaBenchmarkMeter
}

type daytonaBenchmarkMeter struct {
	mu      sync.Mutex
	created int
	deleted int
	seconds float64
	max     int
}

func TestDaytonaBenchmarkSnapshotPreflight(t *testing.T) {
	if os.Getenv("CODEGYM_RUN_AGENT_RUNTIME_DAYTONA_PREFLIGHT") != "1" {
		t.Skip("set CODEGYM_RUN_AGENT_RUNTIME_DAYTONA_PREFLIGHT=1 to verify the shared benchmark snapshot")
	}
	suite := newDaytonaBenchmarkSuite(t)
	if err := suite.meter.beforeCreate(); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Now()
	sandbox, err := suite.client.Create(t.Context(), types.SnapshotParams{
		Snapshot: daytonaBenchmarkSnapshot,
		SandboxBaseParams: types.SandboxBaseParams{
			Labels: map[string]string{
				"codegym": "agent-runtime-benchmark", "benchmark": daytonaBenchmarkLabel,
				"runtime": "preflight", "fixture": "toolchains",
			},
			NetworkBlockAll: false,
			Ephemeral:       true,
		},
	})
	if err != nil {
		t.Fatalf("create Daytona benchmark preflight sandbox: %v", err)
	}
	suite.meter.recordCreate()
	deleted := false
	defer func() {
		if !deleted {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if deleteErr := sandbox.DeleteWithTimeout(cleanupCtx, 30*time.Second); deleteErr != nil {
				t.Errorf("delete Daytona benchmark preflight sandbox: %v", deleteErr)
			} else {
				suite.meter.recordDelete(time.Since(createdAt))
			}
		}
		suite.AssertNoLabeledSandboxes(t)
	}()
	result, err := sandbox.Process.ExecuteCommand(t.Context(), "go version && node --version && npm --version", options.WithExecuteTimeout(30*time.Second))
	if err != nil {
		t.Fatalf("execute Daytona benchmark preflight: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Daytona benchmark preflight exited %d: %s", result.ExitCode, result.Result)
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := sandbox.DeleteWithTimeout(cleanupCtx, 30*time.Second); err != nil {
		t.Fatalf("delete Daytona benchmark preflight sandbox: %v", err)
	}
	suite.meter.recordDelete(time.Since(createdAt))
	deleted = true
	t.Logf("snapshot=%s toolchains=%s", daytonaBenchmarkSnapshot, strings.TrimSpace(result.Result))
}

func newDaytonaBenchmarkSuite(t *testing.T) *daytonaBenchmarkSuite {
	t.Helper()
	apiKey := strings.TrimSpace(os.Getenv("DAYTONA_API_KEY"))
	if apiKey == "" {
		t.Skip("set DAYTONA_API_KEY via Doppler to run the Daytona agent-runtime benchmark")
	}
	client, err := daytona.NewClientWithConfig(&types.DaytonaConfig{APIKey: apiKey, APIUrl: os.Getenv("DAYTONA_API_URL")})
	if err != nil {
		t.Fatalf("create Daytona benchmark client: %v", err)
	}
	return &daytonaBenchmarkSuite{client: client, meter: &daytonaBenchmarkMeter{max: DefaultBenchmarkMaxSandboxes}}
}

func (s *daytonaBenchmarkSuite) VerifierFactory(fixture BenchmarkFixture) Verifier {
	return daytonaBenchmarkVerifier{suite: s, fixture: fixture}
}

func (s *daytonaBenchmarkSuite) Counts() (int, int) {
	return s.meter.counts()
}

func (s *daytonaBenchmarkSuite) Seconds() float64 {
	return s.meter.sandboxSeconds()
}

func (s *daytonaBenchmarkSuite) AssertNoLabeledSandboxes(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		iterator := s.client.List(t.Context(), &daytona.ListSandboxesQuery{Labels: map[string]string{
			"codegym": "agent-runtime-benchmark", "benchmark": daytonaBenchmarkLabel,
		}})
		remaining := make([]string, 0)
		for iterator.Next() {
			sandbox := iterator.Value()
			remaining = append(remaining, sandbox.ID)
		}
		if err := iterator.Err(); err != nil {
			t.Fatalf("list labelled Daytona benchmark sandboxes: %v", err)
		}
		if len(remaining) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("labelled Daytona benchmark sandboxes remain after delete readback: %v", remaining)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (m *daytonaBenchmarkMeter) beforeCreate() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.created >= m.max {
		return fmt.Errorf("agentruntime: Daytona benchmark sandbox cap reached: created=%d max=%d", m.created, m.max)
	}
	return nil
}

func (m *daytonaBenchmarkMeter) recordCreate() {
	m.mu.Lock()
	m.created++
	m.mu.Unlock()
}

func (m *daytonaBenchmarkMeter) recordDelete(lifetime time.Duration) {
	m.mu.Lock()
	m.deleted++
	m.seconds += lifetime.Seconds()
	m.mu.Unlock()
}

func (m *daytonaBenchmarkMeter) counts() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.created, m.deleted
}

func (m *daytonaBenchmarkMeter) sandboxSeconds() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.seconds
}

type daytonaBenchmarkVerifier struct {
	suite   *daytonaBenchmarkSuite
	fixture BenchmarkFixture
}

func (v daytonaBenchmarkVerifier) Verify(ctx context.Context, task TaskSpec, result RunResult) (verification Verification, err error) {
	if result.Manifest.Status != ManifestPresent {
		return (FixtureVerifier{Fixture: v.fixture}).Verify(ctx, task, result)
	}
	violations, err := scanFixtureWorkspace(task.WorkingDirectory)
	if err != nil {
		return Verification{}, err
	}
	if len(violations) > 0 {
		return Verification{Passed: false, Detail: violations[0], PolicyViolations: violations}, nil
	}
	if err := v.suite.meter.beforeCreate(); err != nil {
		return Verification{}, err
	}
	createdAt := time.Now()
	sandbox, err := v.suite.client.Create(ctx, types.SnapshotParams{
		Snapshot: daytonaBenchmarkSnapshot,
		SandboxBaseParams: types.SandboxBaseParams{
			Labels: map[string]string{
				"codegym": "agent-runtime-benchmark", "benchmark": daytonaBenchmarkLabel,
				"runtime": result.Telemetry.Runtime, "fixture": v.fixture.ID,
			},
			NetworkBlockAll: false,
			Ephemeral:       true,
		},
	})
	if err != nil {
		return Verification{}, fmt.Errorf("agentruntime: create Daytona benchmark sandbox: %w", err)
	}
	v.suite.meter.recordCreate()
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if deleteErr := sandbox.DeleteWithTimeout(cleanupCtx, 30*time.Second); deleteErr != nil {
			err = errors.Join(err, fmt.Errorf("agentruntime: delete Daytona benchmark sandbox %s: %w", sandbox.ID, deleteErr))
			return
		}
		v.suite.meter.recordDelete(time.Since(createdAt))
	}()

	files, err := benchmarkCandidateFiles(task.WorkingDirectory, v.fixture.ID)
	if err != nil {
		return Verification{}, err
	}
	if err := uploadBenchmarkFiles(ctx, sandbox, files); err != nil {
		return Verification{}, err
	}
	setupCommand, probeCommand, err := daytonaFixtureCommands(v.fixture)
	if err != nil {
		return Verification{}, err
	}
	setup, err := sandbox.Process.ExecuteCommand(ctx, setupCommand, options.WithExecuteTimeout(2*time.Minute))
	if err != nil {
		return Verification{}, fmt.Errorf("agentruntime: Daytona dependency/build command: %w", err)
	}
	if setup.ExitCode != 0 {
		return Verification{Passed: false, Detail: fmt.Sprintf("Daytona dependency/build failed (exit %d): %s", setup.ExitCode, boundedDiagnostic(setup.Result))}, nil
	}
	blocked := true
	probeNetwork := "network-blocked"
	if err := sandbox.UpdateNetworkSettings(ctx, apiclient.UpdateSandboxNetworkSettings{NetworkBlockAll: &blocked}); err != nil {
		if !isDaytonaTierNetworkRestriction(err) {
			return Verification{}, fmt.Errorf("agentruntime: block Daytona benchmark runtime network: %w", err)
		}
		probeNetwork = "organization-restricted-network"
	}
	probe, err := sandbox.Process.ExecuteCommand(ctx, probeCommand, options.WithExecuteTimeout(45*time.Second))
	if err != nil {
		return Verification{}, fmt.Errorf("agentruntime: Daytona black-box probe: %w", err)
	}
	if probe.ExitCode != 0 {
		return Verification{Passed: false, Detail: fmt.Sprintf("Daytona %s probe failed (exit %d): %s", probeNetwork, probe.ExitCode, boundedDiagnostic(probe.Result))}, nil
	}
	return Verification{Passed: true, Detail: fmt.Sprintf("real dependency resolution and %s Daytona HTTP probe passed", probeNetwork)}, nil
}

func isDaytonaTierNetworkRestriction(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Network access is restricted and cannot be overridden at the sandbox level")
}

func TestDaytonaTierNetworkRestriction(t *testing.T) {
	t.Parallel()
	if !isDaytonaTierNetworkRestriction(errors.New("Validation error: Network access is restricted and cannot be overridden at the sandbox level")) {
		t.Fatal("expected the documented organization-tier restriction to be recognized")
	}
	if isDaytonaTierNetworkRestriction(errors.New("permission denied")) {
		t.Fatal("unexpected network-setting errors must still fail closed")
	}
}

type daytonaBenchmarkFile struct {
	path    string
	content []byte
}

func benchmarkCandidateFiles(root, fixtureID string) ([]daytonaBenchmarkFile, error) {
	files := make([]daytonaBenchmarkFile, 0)
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relative == ".git" || relative == ".codegym" || relative == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !benchmarkCandidatePath(fixtureID, filepath.ToSlash(relative)) {
			return nil
		}
		payload, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		files = append(files, daytonaBenchmarkFile{path: filepath.ToSlash(relative), content: payload})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, err
}

func benchmarkCandidatePath(fixtureID, candidate string) bool {
	switch fixtureID {
	case "go-net-http":
		return candidate == "go.mod" || candidate == "go.sum" || strings.HasSuffix(candidate, ".go")
	case "express":
		base := path.Base(candidate)
		return base == "package.json" || base == "package-lock.json" ||
			strings.HasSuffix(candidate, ".js") || strings.HasSuffix(candidate, ".mjs") || strings.HasSuffix(candidate, ".cjs")
	default:
		return false
	}
}

func uploadBenchmarkFiles(ctx context.Context, sandbox *daytona.Sandbox, files []daytonaBenchmarkFile) error {
	if err := sandbox.FileSystem.CreateFolder(ctx, "work"); err != nil {
		return fmt.Errorf("agentruntime: create Daytona benchmark work directory: %w", err)
	}
	directories := map[string]struct{}{}
	for _, file := range files {
		for directory := path.Dir(file.path); directory != "."; directory = path.Dir(directory) {
			directories[directory] = struct{}{}
		}
	}
	orderedDirectories := make([]string, 0, len(directories))
	for directory := range directories {
		orderedDirectories = append(orderedDirectories, directory)
	}
	sort.Slice(orderedDirectories, func(i, j int) bool {
		leftDepth := strings.Count(orderedDirectories[i], "/")
		rightDepth := strings.Count(orderedDirectories[j], "/")
		return leftDepth < rightDepth || leftDepth == rightDepth && orderedDirectories[i] < orderedDirectories[j]
	})
	for _, directory := range orderedDirectories {
		if err := sandbox.FileSystem.CreateFolder(ctx, "work/"+directory); err != nil {
			return fmt.Errorf("agentruntime: create Daytona benchmark directory %q: %w", directory, err)
		}
	}
	for _, file := range files {
		if err := sandbox.FileSystem.UploadFile(ctx, file.content, "work/"+file.path); err != nil {
			return fmt.Errorf("agentruntime: upload Daytona benchmark file %q: %w", file.path, err)
		}
	}
	return nil
}

func daytonaFixtureCommands(fixture BenchmarkFixture) (string, string, error) {
	probe := fmt.Sprintf(`set -eu
cd "$HOME/work"
PORT=18080 %s > /tmp/codegym-agent-service.log 2>&1 &
pid=$!
trap 'kill "$pid" 2>/dev/null || true' EXIT
for attempt in $(seq 1 80); do
  if body=$(curl --fail --silent --show-error "http://127.0.0.1:18080%s"); then
    python3 -c 'import json,sys; assert json.loads(sys.argv[1]) == json.loads(sys.argv[2])' "$body" '%s'
    exit 0
  fi
  sleep 0.05
done
cat /tmp/codegym-agent-service.log >&2
exit 1`, fixtureLaunchCommand(fixture.ID), fixture.EndpointPath, fixture.ExpectedPayload)
	switch fixture.ID {
	case "go-net-http":
		return `set -eu
cd "$HOME/work"
go mod download
go mod verify
go test ./...
go build -o /tmp/codegym-agent-service .`, probe, nil
	case "express":
		return `set -eu
cd "$HOME/work"
npm install --ignore-scripts --no-audit --no-fund
test "$(node -p "require('./node_modules/express/package.json').version")" = "5.1.0"
node --check server.js`, probe, nil
	default:
		return "", "", fmt.Errorf("agentruntime: unsupported Daytona benchmark fixture %q", fixture.ID)
	}
}

func fixtureLaunchCommand(fixtureID string) string {
	if fixtureID == "go-net-http" {
		return "/tmp/codegym-agent-service"
	}
	return "node server.js"
}

func boundedDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4096 {
		return value
	}
	return value[:4096] + "<truncated>"
}
