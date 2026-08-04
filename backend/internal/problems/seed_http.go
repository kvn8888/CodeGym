package problems

import (
	"encoding/json"
	"fmt"

	"github.com/kvn8888/codegym/backend/internal/execution"
)

const (
	goHTTPItemsID = "go-http-items"

	goHTTPItemsSkeleton = `package main

import (
	"net/http"
	"os"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /items", createItem)
	mux.HandleFunc("GET /items/{id}", getItem)
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), mux); err != nil {
		panic(err)
	}
}

func createItem(response http.ResponseWriter, request *http.Request) {
	// TODO: decode the JSON body, store the item, and return it with status 201.
	http.Error(response, "not implemented", http.StatusNotImplemented)
}

func getItem(response http.ResponseWriter, request *http.Request) {
	// TODO: return the item identified by request.PathValue("id"), or 404.
	http.Error(response, "not implemented", http.StatusNotImplemented)
}
`

	goHTTPItemsReference = `package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"sync"
)

var (
	itemsMu sync.Mutex
	items = map[string]map[string]any{}
	nextID = 1
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /items", createItem)
	mux.HandleFunc("GET /items/{id}", getItem)
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), mux); err != nil {
		panic(err)
	}
}

func createItem(response http.ResponseWriter, request *http.Request) {
	var input struct {
		SKU string
		Qty int
	}
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input.SKU == "" || input.Qty < 1 {
		http.Error(response, "invalid item", http.StatusBadRequest)
		return
	}
	itemsMu.Lock()
	id := strconv.Itoa(nextID)
	nextID++
	created := map[string]any{"id": id, "sku": input.SKU, "qty": input.Qty}
	items[id] = created
	itemsMu.Unlock()
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(response).Encode(created)
}

func getItem(response http.ResponseWriter, request *http.Request) {
	itemsMu.Lock()
	found, ok := items[request.PathValue("id")]
	itemsMu.Unlock()
	if !ok {
		http.NotFound(response, request)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(found)
}
`
)

func goHTTPItemsDefinition() (Definition, error) {
	config, cases, err := NormalizeHTTPCases(TestConfig{
		Strategy:   TestStrategyHTTP,
		Comparator: Comparator{Kind: ComparatorExact},
	}, []HTTPCase{
		{
			Name: "creates-an-item",
			Request: HTTPRequest{
				Method: "POST", Path: "/items",
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    json.RawMessage(`{"sku":"abc","qty":2}`),
			},
			Expect: HTTPExpectation{
				Status: 201, JSON: json.RawMessage(`{"id":"1","sku":"abc","qty":2}`),
				Headers: map[string]string{"Content-Type": "application/json"},
			},
		},
		{
			Name:    "gets-the-created-item",
			Request: HTTPRequest{Method: "GET", Path: "/items/1"},
			Expect:  HTTPExpectation{Status: 200, JSON: json.RawMessage(`{"qty":2,"sku":"abc","id":"1"}`)},
		},
		{
			Name:    "returns-not-found-for-an-unknown-item",
			Request: HTTPRequest{Method: "GET", Path: "/items/404"},
			Expect:  HTTPExpectation{Status: 404, Body: stringPointer("404 page not found\n")},
		},
		{
			Name: "increments-item-identifiers",
			Request: HTTPRequest{
				Method: "POST", Path: "/items",
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    json.RawMessage(`{"sku":"xyz","qty":1}`),
			},
			Expect: HTTPExpectation{Status: 201, JSON: json.RawMessage(`{"id":"2","sku":"xyz","qty":1}`)},
		},
	})
	if err != nil {
		return Definition{}, fmt.Errorf("build Go HTTP seed cases: %w", err)
	}
	harnessConfig, err := json.Marshal(struct {
		ReadinessTimeoutSeconds int        `json:"readiness_timeout_seconds"`
		Cases                   []HTTPCase `json:"cases"`
	}{ReadinessTimeoutSeconds: config.ReadinessTimeoutSeconds, Cases: cases})
	if err != nil {
		return Definition{}, fmt.Errorf("encode Go HTTP seed cases: %w", err)
	}
	return Definition{
		Problem: Problem{
			Summary: Summary{
				ID: goHTTPItemsID, Title: "In-Memory Items API", Category: "backend",
				Language: "go", Framework: "net/http", Difficulty: 2,
				Tags: []string{"go", "http", "json", "state"}, EstimatedMinutes: 35, Type: "coding",
			},
			Version: "1.0.0", Subcategory: "http-servers",
			Description: `Build a small in-memory JSON API with two routes:

- POST /items accepts {"sku":"...","qty":N}, stores a new item, and returns it with status 201.
- GET /items/{id} returns a stored item with status 200 or a standard 404 response when it does not exist.

Read the server port from the PORT environment variable. Use only the Go standard library. Keep state safe for concurrent requests.`,
			Runtime:    Runtime{Image: "go1.25.4", TimeoutSeconds: 30, MemoryMB: 1024, NetworkMode: "block-all"},
			Files:      FileManifest{Skeleton: []FileRef{{Path: "main.go", Entry: true}}},
			TestConfig: config,
			Hints: []Hint{
				{Cost: 0, Text: "Use http.ServeMux method-and-path patterns and request.PathValue for the id."},
				{Cost: 1, Text: "Guard the item map and next id with a mutex, then encode responses with json.NewEncoder."},
			},
		},
		SkeletonFiles: []File{{Path: "main.go", Content: goHTTPItemsSkeleton}},
		HiddenTestFiles: []File{
			{Path: "codegym_http_harness.go", Content: execution.GoHTTPHarnessSource},
			{Path: "codegym_http_comparator.go", Content: execution.GoComparatorSource},
			{Path: "codegym_http_cases.json", Content: string(harnessConfig)},
			{Path: "codegym_http_compile.py", Content: execution.GoHTTPCompileRunnerSource},
		},
		ReferenceSolution: goHTTPItemsReference,
		Entrypoint:        "codegym_http_compile.py",
		Visibility:        VisibilityGlobal,
	}, nil
}

func stringPointer(value string) *string {
	return &value
}
