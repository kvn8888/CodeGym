package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	protocolSchema              = 1
	defaultReadinessTimeout     = 10 * time.Second
	caseTimeout                 = 5 * time.Second
	readinessPollInterval       = 25 * time.Millisecond
	serverExitObservationWindow = 100 * time.Millisecond
	maxResponseBodyBytes        = 1 << 20
	maxDiagnosticBodyBytes      = 4 << 10
)

type harnessConfig struct {
	ReadinessTimeoutSeconds int        `json:"readiness_timeout_seconds"`
	Cases                   []httpCase `json:"cases"`
}

type httpCase struct {
	Name       string          `json:"name"`
	Request    httpRequest     `json:"request"`
	Expect     httpExpectation `json:"expect"`
	Comparator comparator      `json:"comparator"`
}

type httpRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

type httpExpectation struct {
	Status  int               `json:"status"`
	JSON    json.RawMessage   `json:"json"`
	Headers map[string]string `json:"headers"`
	Body    *string           `json:"body"`
}

type comparator struct {
	Kind    string   `json:"kind"`
	Epsilon *float64 `json:"epsilon"`
}

type caseResult struct {
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	DurationMS int64   `json:"duration_ms"`
	Error      *string `json:"error"`
}

type serverState struct {
	command *exec.Cmd
	wait    <-chan error
	exited  bool
	err     error
}

func (state *serverState) observe(timeout time.Duration) bool {
	if state.exited {
		return true
	}
	if timeout <= 0 {
		select {
		case state.err = <-state.wait:
			state.exited = true
		default:
		}
		return state.exited
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case state.err = <-state.wait:
		state.exited = true
	case <-timer.C:
	}
	return state.exited
}

func (state *serverState) describeExit() string {
	if state.err == nil {
		return "server exited normally"
	}
	return "server exited: " + state.err.Error()
}

func appendEvent(event map[string]any) error {
	stream, err := os.OpenFile(".codegym/cases.jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(stream).Encode(event); err != nil {
		_ = stream.Close()
		return err
	}
	if err := stream.Sync(); err != nil {
		_ = stream.Close()
		return err
	}
	return stream.Close()
}

func writeVerdict(status string, cases []caseResult) error {
	payload := map[string]any{"schema": protocolSchema, "status": status, "compile_error": nil, "cases": cases}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := os.WriteFile(".codegym/verdict.json.tmp", data, 0o600); err != nil {
		return err
	}
	return os.Rename(".codegym/verdict.json.tmp", ".codegym/verdict.json")
}

func recordResult(results *[]caseResult, name, status string, started time.Time, detail *string) error {
	result := caseResult{
		Name: name, Status: status,
		DurationMS: max(int64(0), time.Since(started).Milliseconds()), Error: detail,
	}
	*results = append(*results, result)
	return appendEvent(map[string]any{
		"event": "case_result", "name": result.Name, "status": result.Status,
		"duration_ms": result.DurationMS, "error": result.Error,
	})
}

func lifecycleFailure(results *[]caseResult, name, detail string, started time.Time) error {
	if err := appendEvent(map[string]any{"event": "case_start", "name": name}); err != nil {
		return err
	}
	return recordResult(results, name, "fail", started, &detail)
}

func choosePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, err
	}
	return port, nil
}

func serverEnvironment(port int) []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PORT=") {
			environment = append(environment, entry)
		}
	}
	return append(environment, "PORT="+strconv.Itoa(port))
}

func startServer(binary string, port int) (*serverState, error) {
	command := exec.Command(binary)
	command.Env = serverEnvironment(port)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	return &serverState{command: command, wait: wait}, nil
}

func stopServer(state *serverState) {
	if state == nil || state.command == nil || state.command.Process == nil {
		return
	}
	// Kill the process group even when the leader already exited so children
	// inherited from the learner server cannot outlive the harness.
	_ = syscall.Kill(-state.command.Process.Pid, syscall.SIGKILL)
	if !state.exited {
		state.observe(2 * time.Second)
	}
}

func waitUntilReady(state *serverState, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastError error
	for {
		if state.observe(0) {
			return errors.New(state.describeExit() + " before accepting connections")
		}
		connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return nil
		}
		lastError = err
		if time.Now().After(deadline) {
			return fmt.Errorf("server never became ready within %s on PORT=%d (last connection error: %v)", timeout, port, lastError)
		}
		time.Sleep(readinessPollInterval)
	}
}

func decodeJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("body contains more than one JSON value")
		}
		return nil, err
	}
	return value, nil
}

func readResponseBody(stream io.Reader) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(stream, maxResponseBodyBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) > maxResponseBodyBytes {
		return data[:maxResponseBodyBytes], true, nil
	}
	return data, false, nil
}

func diagnosticBody(data []byte, responseTruncated bool) string {
	truncated := responseTruncated || len(data) > maxDiagnosticBodyBytes
	if len(data) > maxDiagnosticBodyBytes {
		data = data[:maxDiagnosticBodyBytes]
	}
	text := strconv.QuoteToASCII(string(data))
	if truncated {
		text += "...[truncated]"
	}
	return text
}

func expectedBody(expect httpExpectation) string {
	if len(expect.JSON) > 0 {
		return "json=" + string(expect.JSON)
	}
	if expect.Body != nil {
		return "body=" + strconv.QuoteToASCII(*expect.Body)
	}
	return "body=<not asserted>"
}

func compareResponse(testCase httpCase, response *http.Response, body []byte, responseTruncated bool) *string {
	failures := make([]string, 0, 4)
	if response.StatusCode != testCase.Expect.Status {
		failures = append(failures, fmt.Sprintf("status expected %d, received %d", testCase.Expect.Status, response.StatusCode))
	}
	for name, expected := range testCase.Expect.Headers {
		if actual := response.Header.Get(name); actual != expected {
			failures = append(failures, fmt.Sprintf("header %s expected %q, received %q", name, expected, actual))
		}
	}
	if responseTruncated {
		failures = append(failures, fmt.Sprintf("response body exceeded %d bytes", maxResponseBodyBytes))
	} else if len(testCase.Expect.JSON) > 0 {
		expected, expectedErr := decodeJSON(testCase.Expect.JSON)
		actual, actualErr := decodeJSON(body)
		if expectedErr != nil {
			failures = append(failures, "hidden expected JSON is invalid: "+expectedErr.Error())
		} else if actualErr != nil {
			failures = append(failures, "response body is not valid JSON: "+actualErr.Error())
		} else {
			equal, compareErr := compareValues(testCase.Comparator.Kind, expected, actual, testCase.Comparator.Epsilon)
			if compareErr != nil {
				failures = append(failures, "compare JSON: "+compareErr.Error())
			} else if !equal {
				failures = append(failures, "JSON body did not satisfy "+testCase.Comparator.Kind+" comparator")
			}
		}
	} else if testCase.Expect.Body != nil && string(body) != *testCase.Expect.Body {
		failures = append(failures, "raw response body did not match")
	}
	if len(failures) == 0 {
		return nil
	}
	detail := fmt.Sprintf(
		"expected status=%d headers=%v %s; received status=%d headers=%v body=%s; %s",
		testCase.Expect.Status, testCase.Expect.Headers, expectedBody(testCase.Expect),
		response.StatusCode, response.Header, diagnosticBody(body, responseTruncated), strings.Join(failures, "; "),
	)
	return &detail
}

func runCase(client *http.Client, baseURL string, testCase httpCase) (*string, error) {
	var body io.Reader
	if len(testCase.Request.Body) > 0 {
		body = bytes.NewReader(testCase.Request.Body)
	}
	request, err := http.NewRequest(testCase.Request.Method, baseURL+testCase.Request.Path, body)
	if err != nil {
		return nil, err
	}
	for name, value := range testCase.Request.Headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseBody, truncated, err := readResponseBody(response.Body)
	if err != nil {
		return nil, err
	}
	return compareResponse(testCase, response, responseBody, truncated), nil
}

func loadConfig(path string) (harnessConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return harnessConfig{}, err
	}
	var config harnessConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return harnessConfig{}, err
	}
	if config.ReadinessTimeoutSeconds <= 0 {
		config.ReadinessTimeoutSeconds = int(defaultReadinessTimeout / time.Second)
	}
	if len(config.Cases) == 0 {
		return harnessConfig{}, errors.New("http case config is empty")
	}
	return config, nil
}

func main() {
	serverPath := flag.String("server", "", "compiled learner server")
	configPath := flag.String("config", "", "hidden HTTP case config")
	flag.Parse()
	if *serverPath == "" || *configPath == "" {
		fmt.Fprintln(os.Stderr, "server and config are required")
		os.Exit(2)
	}
	config, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load HTTP harness config:", err)
		os.Exit(2)
	}
	port, err := choosePort()
	if err != nil {
		fmt.Fprintln(os.Stderr, "choose HTTP server port:", err)
		os.Exit(2)
	}
	state, err := startServer(*serverPath, port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start learner HTTP server:", err)
		os.Exit(2)
	}
	defer stopServer(state)

	results := make([]caseResult, 0, len(config.Cases))
	readinessStarted := time.Now()
	readinessTimeout := time.Duration(config.ReadinessTimeoutSeconds) * time.Second
	if err := waitUntilReady(state, port, readinessTimeout); err != nil {
		detail := "HTTP server readiness failed: " + err.Error()
		if eventErr := lifecycleFailure(&results, "server readiness", detail, readinessStarted); eventErr != nil {
			fmt.Fprintln(os.Stderr, "record readiness failure:", eventErr)
			os.Exit(2)
		}
		if err := writeVerdict("failed", results); err != nil {
			fmt.Fprintln(os.Stderr, "write readiness verdict:", err)
			os.Exit(2)
		}
		return
	}

	client := &http.Client{Timeout: caseTimeout}
	baseURL := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	allPassed := true
	for index, testCase := range config.Cases {
		if state.observe(0) {
			detail := fmt.Sprintf("HTTP server died mid-suite after %d of %d cases: %s", index, len(config.Cases), state.describeExit())
			if err := lifecycleFailure(&results, "server lifecycle", detail, time.Now()); err != nil {
				fmt.Fprintln(os.Stderr, "record server lifecycle failure:", err)
				os.Exit(2)
			}
			allPassed = false
			break
		}

		started := time.Now()
		if err := appendEvent(map[string]any{"event": "case_start", "name": testCase.Name}); err != nil {
			fmt.Fprintln(os.Stderr, "record HTTP case start:", err)
			os.Exit(2)
		}
		detail, requestErr := runCase(client, baseURL, testCase)
		if requestErr != nil {
			if state.observe(serverExitObservationWindow) {
				message := fmt.Sprintf("HTTP server died mid-suite while running case %q: %s", testCase.Name, state.describeExit())
				detail = &message
			} else {
				message := "HTTP request failed: " + requestErr.Error()
				detail = &message
			}
		}
		status := "pass"
		if detail != nil {
			status = "fail"
			allPassed = false
		}
		if err := recordResult(&results, testCase.Name, status, started, detail); err != nil {
			fmt.Fprintln(os.Stderr, "record HTTP case result:", err)
			os.Exit(2)
		}
		if requestErr != nil && state.exited {
			break
		}
	}

	status := "failed"
	if allPassed {
		status = "passed"
	}
	if err := writeVerdict(status, results); err != nil {
		fmt.Fprintln(os.Stderr, "write HTTP verdict:", err)
		os.Exit(2)
	}
}
