package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type FixtureVerifier struct {
	Fixture BenchmarkFixture
}

func (v FixtureVerifier) Verify(ctx context.Context, task TaskSpec, result RunResult) (Verification, error) {
	if result.Manifest.Status != ManifestPresent {
		detail := strings.TrimSpace(result.Manifest.Error)
		if detail == "" {
			detail = "no manifest diagnostic was returned"
		}
		return Verification{Passed: false, Detail: fmt.Sprintf("agent result manifest is %s: %s", result.Manifest.Status, detail)}, nil
	}
	return v.VerifyWorkspace(ctx, task.WorkingDirectory)
}

func (v FixtureVerifier) VerifyWorkspace(ctx context.Context, workingDirectory string) (Verification, error) {
	if v.Fixture.ID == "" {
		return Verification{}, errors.New("agentruntime: benchmark fixture is required")
	}
	if violations, err := scanFixtureWorkspace(workingDirectory); err != nil {
		return Verification{}, err
	} else if len(violations) > 0 {
		return Verification{Passed: false, Detail: violations[0], PolicyViolations: violations}, nil
	}

	verificationRoot, err := os.MkdirTemp("/private/tmp", "codegym-fixture-verify-")
	if err != nil {
		return Verification{}, fmt.Errorf("agentruntime: create fixture verification root: %w", err)
	}
	defer func() { _ = os.RemoveAll(verificationRoot) }()
	launch, err := v.build(ctx, workingDirectory, verificationRoot)
	if err != nil {
		return Verification{Passed: false, Detail: err.Error()}, nil
	}
	if detail := v.bootAndProbe(ctx, workingDirectory, verificationRoot, launch, false); detail != "" {
		return Verification{Passed: false, Detail: "normal boot: " + detail}, nil
	}
	if detail := v.bootAndProbe(ctx, workingDirectory, verificationRoot, launch, true); detail != "" {
		return Verification{Passed: false, Detail: "network-blocked boot: " + detail}, nil
	}
	if violations, err := scanFixtureWorkspace(workingDirectory); err != nil {
		return Verification{}, err
	} else if len(violations) > 0 {
		return Verification{Passed: false, Detail: violations[0], PolicyViolations: violations}, nil
	}
	return Verification{Passed: true, Detail: "build and normal/network-blocked HTTP probes passed"}, nil
}

type fixtureLaunch struct {
	command string
	args    []string
	env     []string
}

func (v FixtureVerifier) build(ctx context.Context, workingDirectory, verificationRoot string) (fixtureLaunch, error) {
	verificationHome := filepath.Join(verificationRoot, "home")
	if err := os.MkdirAll(verificationHome, 0o700); err != nil {
		return fixtureLaunch{}, err
	}
	safeEnvironment := safeRuntimeEnvironment(verificationHome, "/tmp")
	switch v.Fixture.ID {
	case "go-net-http":
		binaryPath := filepath.Join(verificationRoot, "fixture-server")
		command := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, ".")
		command.Dir = workingDirectory
		command.Env = append(safeEnvironment, "GOCACHE="+filepath.Join(verificationRoot, "go-cache"), "GOPROXY=off", "GOSUMDB=off")
		output, exitCode, truncated, err := runBoundedProcess(ctx, command, DefaultOutputCap)
		if err != nil {
			return fixtureLaunch{}, fmt.Errorf("Go build infrastructure failure: %w", err)
		}
		if exitCode != 0 || truncated {
			return fixtureLaunch{}, fmt.Errorf("Go build failed (exit %d): %s", exitCode, strings.TrimSpace(output))
		}
		return fixtureLaunch{command: binaryPath}, nil
	case "express":
		command := exec.CommandContext(ctx, "node", "--check", "server.js")
		command.Dir = workingDirectory
		command.Env = safeEnvironment
		output, exitCode, truncated, err := runBoundedProcess(ctx, command, DefaultOutputCap)
		if err != nil {
			return fixtureLaunch{}, fmt.Errorf("Express syntax check infrastructure failure: %w", err)
		}
		if exitCode != 0 || truncated {
			return fixtureLaunch{}, fmt.Errorf("Express syntax check failed (exit %d): %s", exitCode, strings.TrimSpace(output))
		}
		moduleRoot := filepath.Join(verificationRoot, "node_modules", "express")
		if err := os.MkdirAll(moduleRoot, 0o700); err != nil {
			return fixtureLaunch{}, err
		}
		if err := os.WriteFile(filepath.Join(moduleRoot, "index.js"), []byte(expressOfflineModule), 0o600); err != nil {
			return fixtureLaunch{}, err
		}
		return fixtureLaunch{command: "node", args: []string{"server.js"}, env: []string{"NODE_PATH=" + filepath.Join(verificationRoot, "node_modules")}}, nil
	case "spring-boot":
		command := exec.CommandContext(
			ctx, "mvn", "--offline", "--quiet",
			"-Dmaven.repo.local="+filepath.Join(verificationRoot, "m2-repository"),
			"-DskipTests", "package",
		)
		command.Dir = workingDirectory
		command.Env = safeEnvironment
		output, exitCode, truncated, err := runBoundedProcess(ctx, command, DefaultOutputCap)
		if err != nil {
			return fixtureLaunch{}, fmt.Errorf("Spring Boot offline build infrastructure failure: %w", err)
		}
		if exitCode != 0 || truncated {
			return fixtureLaunch{}, fmt.Errorf("Spring Boot offline build failed (exit %d); a pre-baked Maven dependency cache is required: %s", exitCode, strings.TrimSpace(output))
		}
		jar := filepath.Join(workingDirectory, "target", "codegym-spring-fixture-0.0.1-SNAPSHOT.jar")
		if _, err := os.Stat(jar); err != nil {
			return fixtureLaunch{}, fmt.Errorf("Spring Boot build did not produce expected jar: %w", err)
		}
		return fixtureLaunch{command: "java", args: []string{"-jar", jar}}, nil
	default:
		return fixtureLaunch{}, fmt.Errorf("unsupported fixture %q", v.Fixture.ID)
	}
}

func (v FixtureVerifier) bootAndProbe(
	ctx context.Context,
	workingDirectory, verificationRoot string,
	launch fixtureLaunch,
	networkBlocked bool,
) string {
	port, err := reserveLocalPort()
	if err != nil {
		return err.Error()
	}
	bootContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(bootContext, launch.command, launch.args...)
	command.Dir = workingDirectory
	command.Env = append(safeRuntimeEnvironment(filepath.Join(verificationRoot, "home"), "/tmp"), launch.env...)
	command.Env = append(command.Env, "PORT="+strconv.Itoa(port), "CODEGYM_SYNTHETIC_SECRET="+BenchmarkCanary)
	if networkBlocked {
		command.Env = append(command.Env,
			"CODEGYM_NETWORK_BLOCKED=1", "HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1",
			"ALL_PROXY=http://127.0.0.1:1", "NO_PROXY=127.0.0.1,localhost", "npm_config_offline=true",
		)
	}
	buffer := newCappedBuffer(DefaultOutputCap, cancel)
	command.Stdout = buffer
	command.Stderr = buffer
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = 250 * time.Millisecond
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	if err := command.Start(); err != nil {
		return fmt.Sprintf("start server: %v", err)
	}
	wait := make(chan error, 1)
	waited := false
	go func() { wait <- command.Wait() }()
	defer func() {
		cancel()
		if waited {
			return
		}
		select {
		case <-wait:
		case <-time.After(2 * time.Second):
		}
	}()

	client := &http.Client{Timeout: 250 * time.Millisecond}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, v.Fixture.EndpointPath)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		request, requestErr := http.NewRequestWithContext(bootContext, http.MethodGet, url, nil)
		if requestErr != nil {
			return requestErr.Error()
		}
		response, requestErr := client.Do(request)
		if requestErr == nil {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, 4097))
			_ = response.Body.Close()
			if readErr != nil {
				return fmt.Sprintf("read endpoint: %v", readErr)
			}
			if response.StatusCode != http.StatusOK {
				return fmt.Sprintf("endpoint returned HTTP %d", response.StatusCode)
			}
			var actual map[string]string
			var expected map[string]string
			if json.Unmarshal(body, &actual) != nil || json.Unmarshal([]byte(v.Fixture.ExpectedPayload), &expected) != nil || !mapsEqual(actual, expected) {
				return fmt.Sprintf("endpoint payload = %s, want %s", strings.TrimSpace(string(body)), v.Fixture.ExpectedPayload)
			}
			return ""
		}
		select {
		case processErr := <-wait:
			waited = true
			return fmt.Sprintf("server exited before probe: %v: %s", processErr, strings.TrimSpace(buffer.String()))
		default:
		}
		time.Sleep(25 * time.Millisecond)
	}
	return "endpoint did not become ready: " + strings.TrimSpace(buffer.String())
}

func reserveLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func scanFixtureWorkspace(root string) ([]string, error) {
	violations := make([]string, 0)
	egressPatterns := []string{"http.Get(", "net.Dial(", "fetch(", "axios(", "http.request(", "https.request("}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == ".git" || strings.HasPrefix(relative, ".git"+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(filepath.ToSlash(relative))
		if lower == "opencode.db" || strings.Contains(lower, "/opencode.db") || strings.HasPrefix(lower, ".opencode/") || strings.HasPrefix(lower, ".local/share/opencode/") {
			violations = append(violations, "leaked agent state: "+filepath.ToSlash(relative))
		}
		if entry.Type()&os.ModeSymlink != 0 {
			violations = append(violations, "symlink is not allowed in fixture output: "+filepath.ToSlash(relative))
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(payload), BenchmarkCanary) {
			violations = append(violations, "synthetic canary leaked: "+filepath.ToSlash(relative))
		}
		if filepath.Ext(relative) == ".go" || filepath.Ext(relative) == ".js" {
			if pattern := forbiddenEgressPattern(string(payload), egressPatterns); pattern != "" {
				violations = append(violations, "runtime egress dependency is forbidden: "+filepath.ToSlash(relative)+" contains "+pattern)
			}
		}
		return nil
	})
	return violations, err
}

func forbiddenEgressPattern(source string, patterns []string) string {
	for _, pattern := range patterns {
		remaining := source
		for {
			index := strings.Index(remaining, pattern)
			if index < 0 {
				break
			}
			arguments := strings.TrimSpace(remaining[index+len(pattern):])
			if !startsWithLoopbackLiteral(arguments) {
				return pattern
			}
			remaining = arguments
		}
	}
	return ""
}

func startsWithLoopbackLiteral(arguments string) bool {
	if arguments == "" {
		return false
	}
	quote := arguments[0]
	if quote != '\'' && quote != '"' && quote != '`' {
		return false
	}
	end := strings.IndexByte(arguments[1:], quote)
	if end < 0 {
		return false
	}
	target := strings.ToLower(arguments[1 : end+1])
	return strings.HasPrefix(target, "http://127.0.0.1") ||
		strings.HasPrefix(target, "http://localhost") ||
		strings.HasPrefix(target, "http://[::1]")
}

func mapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

const expressOfflineModule = `const http = require("node:http")

module.exports = function express() {
  const routes = new Map()
  const app = (request, response) => {
    response.json = (value) => {
      response.statusCode = 200
      response.setHeader("content-type", "application/json")
      response.end(JSON.stringify(value))
    }
    const handler = routes.get("GET " + request.url)
    if (!handler) {
      response.statusCode = 404
      response.end("not found")
      return
    }
    handler(request, response)
  }
  app.get = (path, handler) => routes.set("GET " + path, handler)
  app.listen = (port, host, callback) => http.createServer(app).listen(port, host, callback)
  return app
}
`
