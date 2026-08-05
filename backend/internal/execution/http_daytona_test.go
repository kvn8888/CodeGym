package execution_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/problems"
)

const wrongStatusHTTPServer = `package main

import (
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte("{\"id\":\"1\",\"sku\":\"abc\",\"qty\":2}"))
	})
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), nil); err != nil { panic(err) }
}
`

const wrongJSONHTTPServer = `package main

import (
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		_, _ = response.Write([]byte("{\"id\":\"wrong\",\"sku\":\"abc\",\"qty\":2}"))
	})
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), nil); err != nil { panic(err) }
}
`

const reorderedJSONHTTPServer = `package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
)

type item struct { id, sku string; qty int }

func main() {
	items := map[string]item{}
	nextID := 1
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("POST /items", func(response http.ResponseWriter, request *http.Request) {
		input := map[string]any{}
		if json.NewDecoder(request.Body).Decode(&input) != nil { http.Error(response, "bad request", 400); return }
		mu.Lock()
		created := item{id: strconv.Itoa(nextID), sku: input["sku"].(string), qty: int(input["qty"].(float64))}
		nextID++
		items[created.id] = created
		mu.Unlock()
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(response, "{\"qty\":%d,\"sku\":%q,\"id\":%q}\n", created.qty, created.sku, created.id)
	})
	mux.HandleFunc("GET /items/{id}", func(response http.ResponseWriter, request *http.Request) {
		mu.Lock(); found, ok := items[request.PathValue("id")]; mu.Unlock()
		if !ok { http.NotFound(response, request); return }
		response.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(response, "{\"sku\":%q,\"id\":%q,\"qty\":%d}\n", found.sku, found.id, found.qty)
	})
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), mux); err != nil { panic(err) }
}
`

const neverBindingDaytonaHTTPServer = `package main

import "time"

func main() {
	for { time.Sleep(time.Hour) }
}
`

const panickingDaytonaHTTPServer = `package main

import (
	"net"
	"net/http"
	"os"
)

func main() {
	listener, err := net.Listen("tcp", ":"+os.Getenv("PORT"))
	if err != nil { panic(err) }
	requested := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		_, _ = response.Write([]byte("{\"id\":\"1\",\"sku\":\"abc\",\"qty\":2}"))
		close(requested)
	})
	go func() { _ = http.Serve(listener, mux) }()
	<-requested
	panic("server panic after first request")
}
`

const nonCompilingDaytonaHTTPServer = `package main

func main() {
	this does not compile
}
`

// TestHTTPDaytonaEndToEnd is the gated acceptance matrix for the Go HTTP
// strategy. It uses the real seeded harness and registered Go snapshot.
func TestHTTPDaytonaEndToEnd(t *testing.T) {
	apiKey := os.Getenv("DAYTONA_API_KEY")
	if apiKey == "" {
		t.Skip("set DAYTONA_API_KEY via doppler run -p codegym -c dev")
	}
	runner, err := execution.NewDaytonaRunner(apiKey, os.Getenv("DAYTONA_API_URL"))
	if err != nil {
		t.Fatalf("NewDaytonaRunner: %v", err)
	}
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatalf("EnsureSeed: %v", err)
	}
	definition, err := problemService.GetDefinition(context.Background(), "go-http-items")
	if err != nil {
		t.Fatalf("GetDefinition: %v", err)
	}

	t.Run("correct server passes", func(t *testing.T) {
		outcome := runGoDefinition(t, runner, definition, definition.ReferenceSolution, 0)
		if outcome.Result.Status != execution.JudgeStatusPassed || len(outcome.Result.Cases) != 4 {
			t.Fatalf("result = %#v", outcome.Result)
		}
	})

	t.Run("unmodified skeleton fails cleanly", func(t *testing.T) {
		if len(definition.SkeletonFiles) != 1 {
			t.Fatalf("skeleton files = %#v", definition.SkeletonFiles)
		}
		outcome := runGoDefinition(t, runner, definition, definition.SkeletonFiles[0].Content, 0)
		result := namedHTTPCase(t, outcome, "creates-an-item")
		if outcome.Result.Status != execution.JudgeStatusFailed || result.Error == nil ||
			!strings.Contains(*result.Error, "status expected 201, received 501") {
			t.Fatalf("unmodified skeleton returned something other than a clean failed verdict: %#v", outcome.Result)
		}
	})

	t.Run("wrong status names case and values", func(t *testing.T) {
		outcome := runGoDefinition(t, runner, definition, wrongStatusHTTPServer, 0)
		result := namedHTTPCase(t, outcome, "creates-an-item")
		if outcome.Result.Status != execution.JudgeStatusFailed || result.Error == nil ||
			!strings.Contains(*result.Error, "status expected 201, received 200") ||
			!strings.Contains(*result.Error, "expected status=201") || !strings.Contains(*result.Error, "received status=200") {
			t.Fatalf("result = %#v", outcome.Result)
		}
	})

	t.Run("wrong JSON fails with received body", func(t *testing.T) {
		outcome := runGoDefinition(t, runner, definition, wrongJSONHTTPServer, 0)
		result := namedHTTPCase(t, outcome, "creates-an-item")
		if outcome.Result.Status != execution.JudgeStatusFailed || result.Error == nil ||
			!strings.Contains(*result.Error, "JSON body did not satisfy exact comparator") ||
			!strings.Contains(*result.Error, `\"id\":\"wrong\"`) {
			t.Fatalf("result = %#v", outcome.Result)
		}
	})

	t.Run("JSON key order still passes", func(t *testing.T) {
		outcome := runGoDefinition(t, runner, definition, reorderedJSONHTTPServer, 0)
		if outcome.Result.Status != execution.JudgeStatusPassed || len(outcome.Result.Cases) != 4 {
			t.Fatalf("result = %#v", outcome.Result)
		}
	})

	t.Run("server never becomes ready", func(t *testing.T) {
		outcome := runGoDefinition(t, runner, definition, neverBindingDaytonaHTTPServer, 0)
		result := namedHTTPCase(t, outcome, "server readiness")
		if outcome.Result.Status != execution.JudgeStatusFailed || result.Error == nil || !strings.Contains(*result.Error, "never became ready") {
			t.Fatalf("result = %#v", outcome.Result)
		}
	})

	t.Run("server panic is a mid-suite death", func(t *testing.T) {
		outcome := runGoDefinition(t, runner, definition, panickingDaytonaHTTPServer, 0)
		if outcome.Result.Status != execution.JudgeStatusFailed || !httpCasesContainError(outcome.Result.Cases, "died mid-suite") {
			t.Fatalf("result = %#v", outcome.Result)
		}
		if len(outcome.Result.Cases) >= 4 {
			t.Fatalf("server death cascaded through remaining cases: %#v", outcome.Result.Cases)
		}
	})

	t.Run("compile error runs no cases", func(t *testing.T) {
		outcome := runGoDefinition(t, runner, definition, nonCompilingDaytonaHTTPServer, 0)
		if outcome.Result.Status != execution.JudgeStatusFailed || outcome.Result.CompileError == nil || len(outcome.Result.Cases) != 0 {
			t.Fatalf("result = %#v", outcome.Result)
		}
	})
}

func namedHTTPCase(t *testing.T, outcome execution.RunOutcome, name string) execution.CaseResult {
	t.Helper()
	for _, result := range outcome.Result.Cases {
		if result.Name == name {
			return result
		}
	}
	t.Fatalf("case %q not found in %#v", name, outcome.Result.Cases)
	return execution.CaseResult{}
}

func httpCasesContainError(cases []execution.CaseResult, fragment string) bool {
	for _, result := range cases {
		if result.Error != nil && strings.Contains(*result.Error, fragment) {
			return true
		}
	}
	return false
}
