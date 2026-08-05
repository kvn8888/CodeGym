package agentruntime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const BenchmarkCanary = "CODEGYM_SYNTHETIC_CANARY_7e4f2a91"

type FixtureFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type BrokenFixture struct {
	Name  string
	Files []FixtureFile
}

type BenchmarkFixture struct {
	ID              string
	Stack           string
	TaskPrompt      string
	EndpointPath    string
	ExpectedPayload string
	BaseFiles       []FixtureFile
	GoldenFiles     []FixtureFile
	Broken          []BrokenFixture
}

func BenchmarkFixtures() []BenchmarkFixture {
	return []BenchmarkFixture{goHTTPFixture(), expressFixture()}
}

func BenchmarkFixtureByID(id string) (BenchmarkFixture, error) {
	for _, fixture := range BenchmarkFixtures() {
		if fixture.ID == id {
			return fixture, nil
		}
	}
	return BenchmarkFixture{}, fmt.Errorf("agentruntime: unknown benchmark fixture %q", id)
}

func (f BenchmarkFixture) MaterializeBase(directory string) error {
	return materializeFixtureFiles(directory, f.BaseFiles)
}

func (f BenchmarkFixture) MaterializeGolden(directory string) error {
	if err := f.MaterializeBase(directory); err != nil {
		return err
	}
	return materializeFixtureFiles(directory, f.GoldenFiles)
}

func (f BenchmarkFixture) MaterializeBroken(directory, name string) error {
	if err := f.MaterializeBase(directory); err != nil {
		return err
	}
	for _, broken := range f.Broken {
		if broken.Name == name {
			return materializeFixtureFiles(directory, broken.Files)
		}
	}
	return fmt.Errorf("agentruntime: fixture %q has no broken variant %q", f.ID, name)
}

func materializeFixtureFiles(directory string, files []FixtureFile) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fmt.Errorf("agentruntime: open fixture root: %w", err)
	}
	defer func() { _ = root.Close() }()
	for _, file := range files {
		path, err := confinedPath(file.Path)
		if err != nil {
			return err
		}
		if parent := filepath.Dir(path); parent != "." {
			if err := root.MkdirAll(parent, 0o700); err != nil {
				return err
			}
		}
		if err := root.WriteFile(path, []byte(file.Content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func fixtureIDs(fixtures []BenchmarkFixture) []string {
	ids := make([]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		ids = append(ids, fixture.ID)
	}
	sort.Strings(ids)
	return ids
}

func goHTTPFixture() BenchmarkFixture {
	const golden = `package main

import (
	"encoding/json"
	"net/http"
	"os"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "stack": "go"})
	})
	_ = http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"), mux)
}
`
	return BenchmarkFixture{
		ID: "go-net-http", Stack: "Go net/http", EndpointPath: "/health",
		ExpectedPayload: `{"stack":"go","status":"ok"}`,
		TaskPrompt:      "Build a Go net/http service in this workspace. GET /health must return JSON with status=ok and stack=go. Bind to 127.0.0.1:$PORT, use only the standard library, build and test it, and leave no runtime dependency on network egress.",
		BaseFiles:       []FixtureFile{{Path: "go.mod", Content: "module codegym-fixture\n\ngo 1.25.4\n"}},
		GoldenFiles:     []FixtureFile{{Path: "main.go", Content: golden}},
		Broken: []BrokenFixture{
			{Name: "wrong-payload", Files: []FixtureFile{{Path: "main.go", Content: replaceFixture(golden, `"status": "ok"`, `"status": "broken"`)}}},
			{Name: "wrong-endpoint", Files: []FixtureFile{{Path: "main.go", Content: replaceFixture(golden, `"/health"`, `"/ready"`)}}},
			{Name: "canary-leak", Files: []FixtureFile{{Path: "main.go", Content: golden}, {Path: "debug.txt", Content: BenchmarkCanary}}},
			{Name: "agent-state-leak", Files: []FixtureFile{{Path: "main.go", Content: golden}, {Path: ".opencode/opencode.db", Content: "synthetic state"}}},
			{Name: "runtime-egress", Files: []FixtureFile{{Path: "main.go", Content: replaceFixture(golden, "func main() {", "func main() {\n\t_, _ = http.Get(\"https://example.invalid/bootstrap\")")}}},
		},
	}
}

func expressFixture() BenchmarkFixture {
	const golden = `const express = require("express")

const app = express()
app.get("/health", (_request, response) => {
  response.json({ status: "ok", stack: "express" })
})
app.listen(Number(process.env.PORT), "127.0.0.1")
`
	return BenchmarkFixture{
		ID: "express", Stack: "Express", EndpointPath: "/health",
		ExpectedPayload: `{"stack":"express","status":"ok"}`,
		TaskPrompt:      "Build an Express service in this workspace. GET /health must return JSON with status=ok and stack=express. Bind to 127.0.0.1:$PORT, use the pinned express dependency from package.json, validate it, and leave no runtime dependency on network egress.",
		BaseFiles:       []FixtureFile{{Path: "package.json", Content: `{"name":"codegym-express-fixture","private":true,"version":"1.0.0","dependencies":{"express":"5.1.0"}}` + "\n"}},
		GoldenFiles:     []FixtureFile{{Path: "server.js", Content: golden}},
		Broken: []BrokenFixture{
			{Name: "wrong-payload", Files: []FixtureFile{{Path: "server.js", Content: replaceFixture(golden, `status: "ok"`, `status: "broken"`)}}},
			{Name: "wrong-endpoint", Files: []FixtureFile{{Path: "server.js", Content: replaceFixture(golden, `"/health"`, `"/ready"`)}}},
			{Name: "canary-leak", Files: []FixtureFile{{Path: "server.js", Content: golden}, {Path: "debug.txt", Content: BenchmarkCanary}}},
			{Name: "agent-state-leak", Files: []FixtureFile{{Path: "server.js", Content: golden}, {Path: ".local/share/opencode/opencode.db", Content: "synthetic state"}}},
			{Name: "runtime-egress", Files: []FixtureFile{{Path: "server.js", Content: "fetch(\"https://example.invalid/bootstrap\")\n" + golden}}},
		},
	}
}

func replaceFixture(source, oldValue, newValue string) string {
	replaced := stringsReplaceOnce(source, oldValue, newValue)
	if replaced == source {
		panic(errors.New("agentruntime: invalid fixture replacement"))
	}
	return replaced
}

func stringsReplaceOnce(source, oldValue, newValue string) string {
	index := -1
	if oldValue != "" {
		for offset := 0; offset+len(oldValue) <= len(source); offset++ {
			if source[offset:offset+len(oldValue)] == oldValue {
				index = offset
				break
			}
		}
	}
	if index < 0 {
		return source
	}
	return source[:index] + newValue + source[index+len(oldValue):]
}
