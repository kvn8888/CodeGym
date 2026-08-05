package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type localHTTPHarnessConfig struct {
	ReadinessTimeoutSeconds int             `json:"readiness_timeout_seconds"`
	Cases                   []localHTTPCase `json:"cases"`
}

type localHTTPCase struct {
	Name       string               `json:"name"`
	Request    localHTTPRequest     `json:"request"`
	Expect     localHTTPExpectation `json:"expect"`
	Comparator localComparator      `json:"comparator"`
}

type localHTTPRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

type localHTTPExpectation struct {
	Status  int               `json:"status"`
	JSON    json.RawMessage   `json:"json,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    *string           `json:"body,omitempty"`
}

type localComparator struct {
	Kind    string   `json:"kind"`
	Epsilon *float64 `json:"epsilon,omitempty"`
}

const startingHTTPServer = `package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
)

func main() {
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil { panic(err) }
	pids := fmt.Sprintf("%d\n%d\n", os.Getpid(), child.Process.Pid)
	if err := os.WriteFile(".codegym/server.pids", []byte(pids), 0600); err != nil { panic(err) }
	mux := http.NewServeMux()
	mux.HandleFunc("POST /items", func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("X-Request-Id", "request-1")
		response.WriteHeader(http.StatusCreated)
		_, _ = response.Write([]byte("{\"qty\":2,\"id\":\"item-1\"}"))
	})
	mux.HandleFunc("GET /raw", func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write([]byte("ready"))
	})
	if err := http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"), mux); err != nil { panic(err) }
}
`

const neverBindingHTTPServer = `package main

import "time"

func main() {
	for { time.Sleep(time.Hour) }
}
`

const exitingHTTPServer = `package main

import (
	"net/http"
	"os"
	"sync/atomic"
)

func main() {
	var requests atomic.Int32
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if requests.Add(1) == 2 { os.Exit(17) }
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte("{\"ok\":true}"))
	})
	if err := http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"), handler); err != nil { panic(err) }
}
`

func TestHTTPHarnessRunsCasesAndCleansProcessGroup(t *testing.T) {
	rawBody := "ready"
	verdict, progress, root := runLocalHTTPHarness(t, startingHTTPServer, localHTTPHarnessConfig{
		ReadinessTimeoutSeconds: 2,
		Cases: []localHTTPCase{
			{
				Name: "creates-an-item",
				Request: localHTTPRequest{
					Method: "POST", Path: "/items",
					Headers: map[string]string{"Content-Type": "application/json"},
					Body:    json.RawMessage(`{"sku":"abc","qty":2}`),
				},
				Expect: localHTTPExpectation{
					Status: 201, JSON: json.RawMessage(`{"id":"item-1","qty":2}`),
					Headers: map[string]string{"content-type": "application/json", "x-request-id": "request-1"},
				},
				Comparator: localComparator{Kind: "exact"},
			},
			{
				Name:       "returns-raw-body",
				Request:    localHTTPRequest{Method: "GET", Path: "/raw"},
				Expect:     localHTTPExpectation{Status: 200, Body: &rawBody},
				Comparator: localComparator{Kind: "exact"},
			},
		},
	})
	if verdict.Status != JudgeStatusPassed || len(verdict.Cases) != 2 {
		t.Fatalf("verdict = %#v", verdict)
	}
	if lines := strings.Split(strings.TrimSpace(progress), "\n"); len(lines) != 4 {
		t.Fatalf("progress = %q", progress)
	}
	assertRecordedProcessesStopped(t, filepath.Join(root, ".codegym", "server.pids"))
}

func TestHTTPHarnessReportsServerNeverReady(t *testing.T) {
	verdict, progress, _ := runLocalHTTPHarness(t, neverBindingHTTPServer, localHTTPHarnessConfig{
		ReadinessTimeoutSeconds: 1,
		Cases: []localHTTPCase{{
			Name: "unreached-case", Request: localHTTPRequest{Method: "GET", Path: "/"},
			Expect: localHTTPExpectation{Status: 200}, Comparator: localComparator{Kind: "exact"},
		}},
	})
	if verdict.Status != JudgeStatusFailed || len(verdict.Cases) != 1 || verdict.Cases[0].Name != "server readiness" {
		t.Fatalf("verdict = %#v", verdict)
	}
	if verdict.Cases[0].Error == nil || !strings.Contains(*verdict.Cases[0].Error, "never became ready") {
		t.Fatalf("readiness error = %v", verdict.Cases[0].Error)
	}
	if strings.Contains(progress, "unreached-case") {
		t.Fatalf("actual cases must not run before readiness: %q", progress)
	}
}

func TestHTTPHarnessReportsServerDeathWithoutFailureCascade(t *testing.T) {
	cases := make([]localHTTPCase, 3)
	for index := range cases {
		cases[index] = localHTTPCase{
			Name:       fmt.Sprintf("request-%d", index+1),
			Request:    localHTTPRequest{Method: "GET", Path: "/"},
			Expect:     localHTTPExpectation{Status: 200, JSON: json.RawMessage(`{"ok":true}`)},
			Comparator: localComparator{Kind: "exact"},
		}
	}
	verdict, progress, _ := runLocalHTTPHarness(t, exitingHTTPServer, localHTTPHarnessConfig{
		ReadinessTimeoutSeconds: 2,
		Cases:                   cases,
	})
	if verdict.Status != JudgeStatusFailed || len(verdict.Cases) != 2 {
		t.Fatalf("verdict = %#v", verdict)
	}
	if verdict.Cases[1].Error == nil || !strings.Contains(*verdict.Cases[1].Error, "died mid-suite") {
		t.Fatalf("server death error = %v", verdict.Cases[1].Error)
	}
	if strings.Contains(progress, "request-3") {
		t.Fatalf("server death must stop the suite instead of cascading: %q", progress)
	}
}

func TestHTTPHarnessCompileErrorRunsNoCases(t *testing.T) {
	verdict, progress, _ := runLocalHTTPHarness(t, "package main\nfunc main() { missing }\n", localHTTPHarnessConfig{
		ReadinessTimeoutSeconds: 1,
		Cases: []localHTTPCase{{
			Name: "unreached-case", Request: localHTTPRequest{Method: "GET", Path: "/"},
			Expect: localHTTPExpectation{Status: 200}, Comparator: localComparator{Kind: "exact"},
		}},
	})
	if verdict.Status != JudgeStatusFailed || verdict.CompileError == nil || len(verdict.Cases) != 0 {
		t.Fatalf("compile verdict = %#v", verdict)
	}
	if progress != "" {
		t.Fatalf("compile failure progress = %q", progress)
	}
}

func TestHTTPHarnessPrecompiledMatchAndMismatchHaveIdenticalVerdicts(t *testing.T) {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is required for the precompiled HTTP harness test")
	}
	snapshotDir := t.TempDir()
	harnessPath := filepath.Join(snapshotDir, "http_harness.go")
	comparatorPath := filepath.Join(snapshotDir, "http_comparator.go")
	for path, content := range map[string]string{
		harnessPath: GoHTTPHarnessSource, comparatorPath: GoComparatorSource,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write precompiled harness source: %v", err)
		}
	}
	binaryPath := filepath.Join(snapshotDir, "http-harness-"+GoHTTPHarnessSourceHash)
	command := exec.Command(goBinary, "build", "-o", binaryPath, harnessPath, comparatorPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("precompile HTTP harness: %v\n%s", err, output)
	}
	manifestPath := filepath.Join(snapshotDir, "source.sha256")
	if err := os.WriteFile(manifestPath, []byte(GoHTTPHarnessSourceHash+"\n"), 0o600); err != nil {
		t.Fatalf("write matching harness manifest: %v", err)
	}

	readyBody := "ready"
	config := localHTTPHarnessConfig{
		ReadinessTimeoutSeconds: 2,
		Cases: []localHTTPCase{{
			Name: "precompiled-verdict", Request: localHTTPRequest{Method: "GET", Path: "/raw"},
			Expect:     localHTTPExpectation{Status: 200, Body: &readyBody},
			Comparator: localComparator{Kind: "exact"},
		}},
	}
	matched, _, matchedRoot, matchedOutput := runLocalHTTPHarnessWithEnvironment(
		t, startingHTTPServer, config, "CODEGYM_HTTP_HARNESS_DIR="+snapshotDir,
	)
	assertRecordedProcessesStopped(t, filepath.Join(matchedRoot, ".codegym", "server.pids"))
	if !strings.Contains(matchedOutput, "using precompiled snapshot binary hash="+GoHTTPHarnessSourceHash) {
		t.Fatalf("matching run did not prove precompiled use: %s", matchedOutput)
	}

	if err := os.WriteFile(manifestPath, []byte(strings.Repeat("0", 64)+"\n"), 0o600); err != nil {
		t.Fatalf("write mismatched harness manifest: %v", err)
	}
	fallback, _, fallbackRoot, fallbackOutput := runLocalHTTPHarnessWithEnvironment(
		t, startingHTTPServer, config, "CODEGYM_HTTP_HARNESS_DIR="+snapshotDir,
	)
	assertRecordedProcessesStopped(t, filepath.Join(fallbackRoot, ".codegym", "server.pids"))
	if !strings.Contains(fallbackOutput, "snapshot hash mismatch") ||
		!strings.Contains(fallbackOutput, "compiling server-owned harness at run time") {
		t.Fatalf("mismatched run did not prove safe fallback: %s", fallbackOutput)
	}

	zeroCaseDurations(&matched)
	zeroCaseDurations(&fallback)
	if !reflect.DeepEqual(matched, fallback) {
		t.Fatalf("precompiled verdict differs from fallback\nmatched:  %#v\nfallback: %#v", matched, fallback)
	}
}

func zeroCaseDurations(verdict *HarnessVerdict) {
	for index := range verdict.Cases {
		verdict.Cases[index].DurationMs = 0
	}
}

func runLocalHTTPHarness(t *testing.T, serverSource string, config localHTTPHarnessConfig) (HarnessVerdict, string, string) {
	verdict, progress, root, _ := runLocalHTTPHarnessWithEnvironment(t, serverSource, config)
	return verdict, progress, root
}

func runLocalHTTPHarnessWithEnvironment(
	t *testing.T,
	serverSource string,
	config localHTTPHarnessConfig,
	environment ...string,
) (HarnessVerdict, string, string, string) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the local HTTP harness test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is required for the local HTTP harness test")
	}
	root := t.TempDir()
	protocolDir := filepath.Join(root, ".codegym")
	if err := os.Mkdir(protocolDir, 0o700); err != nil {
		t.Fatalf("mkdir protocol directory: %v", err)
	}
	configData, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal HTTP config: %v", err)
	}
	files := map[string]string{
		"main.go":                    serverSource,
		"codegym_http_cases.json":    string(configData),
		"codegym_http_harness.go":    GoHTTPHarnessSource,
		"codegym_http_comparator.go": GoComparatorSource,
		"codegym_http_compile.py":    GoHTTPCompileRunnerSource,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "python3", "codegym_http_compile.py")
	command.Dir = root
	command.Env = append(os.Environ(), environment...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run HTTP harness: %v\n%s", err, output)
	}
	verdictData, err := os.ReadFile(filepath.Join(protocolDir, "verdict.json"))
	if err != nil {
		t.Fatalf("read verdict.json: %v\n%s", err, output)
	}
	var verdict HarnessVerdict
	if err := json.Unmarshal(verdictData, &verdict); err != nil {
		t.Fatalf("decode verdict.json: %v\n%s", err, output)
	}
	if err := validateHarnessVerdict(verdict); err != nil {
		t.Fatalf("invalid verdict: %v\n%s", err, output)
	}
	progressData, _ := os.ReadFile(filepath.Join(protocolDir, "cases.jsonl"))
	return verdict, string(progressData), root, string(output)
}

func assertRecordedProcessesStopped(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read server pids: %v", err)
	}
	for _, field := range strings.Fields(string(data)) {
		pid, err := strconv.Atoi(field)
		if err != nil {
			t.Fatalf("parse pid %q: %v", field, err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for {
			err = syscall.Kill(pid, 0)
			if errors.Is(err, syscall.ESRCH) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("process %d still exists after harness cleanup (kill check: %v)", pid, err)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
