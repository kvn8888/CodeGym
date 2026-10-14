package envbuild

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
)

// writingClient scripts a fake agent that writes the given files on its
// first turn and finishes on its second. One client per fixture technology;
// both drive the identical Prepare function below.
func writingClient(files map[string]string) *fakeCompletionClient {
	client := &fakeCompletionClient{}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	client.respond = func() (agentruntime.CompletionResponse, error) {
		if client.calls > 1 {
			return agentruntime.CompletionResponse{
				Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"},
			}, nil
		}
		calls := make([]agentruntime.ToolCall, 0, len(paths))
		for i, path := range paths {
			arguments, err := json.Marshal(map[string]string{"path": path, "content": files[path]})
			if err != nil {
				return agentruntime.CompletionResponse{}, err
			}
			calls = append(calls, agentruntime.ToolCall{
				ID:   string(rune('a' + i)),
				Type: "function",
				Function: agentruntime.ToolFunction{
					Name:      "write_file",
					Arguments: string(arguments),
				},
			})
		}
		return agentruntime.CompletionResponse{
			Message: agentruntime.ChatMessage{Role: "assistant", ToolCalls: calls},
		}, nil
	}
	return client
}

// fakeVerify mirrors Track B's future consumption of the handoff: schema,
// identity, commands, and a disjoint editable/protected split. It is test
// scaffolding, not verifier implementation.
func fakeVerify(manifest *EnvironmentManifest) (bool, string) {
	if manifest == nil {
		return false, "no manifest"
	}
	if manifest.SchemaVersion != "environment.manifest.v1" {
		return false, "schema version"
	}
	if manifest.Artifact.ID == "" || manifest.Artifact.Version == "" {
		return false, "artifact identity"
	}
	if manifest.Commands.Setup == "" || manifest.Commands.Build == "" || manifest.Commands.Test == "" {
		return false, "commands"
	}
	protected := map[string]bool{}
	for _, file := range manifest.Workspace.Protected {
		protected[file] = true
	}
	for _, file := range manifest.Workspace.LearnerEditable {
		if protected[file] {
			return false, "editable/protected overlap on " + file
		}
	}
	return true, ""
}

func maskRequest(text string, request BuildRequest) string {
	text = strings.ReplaceAll(text, request.Technology, "\x00TECH\x00")
	text = strings.ReplaceAll(text, request.Objective, "\x00OBJ\x00")
	return text
}

func TestGenericPathServesTwoTechnologies(t *testing.T) {
	fixtures := []struct {
		request BuildRequest
		files   map[string]string
	}{
		{
			request: BuildRequest{Technology: "Starship Framework", Objective: "learn navigation"},
			files: map[string]string{
				"ship.js":                "export const ship = 1;",
				"ship.test.js":           "test ship",
				".codegym/commands.json": `{"setup":"npm install","build":"npm run build","test":"npm test","run":"npm start"}`,
			},
		},
		{
			request: BuildRequest{Technology: "Teapot Framework", Objective: "learn pouring"},
			files: map[string]string{
				"pot.py":                 "pot = 1",
				"test_pot.py":            "test pot",
				".codegym/commands.json": `{"setup":"pip install -r requirements.txt","build":"python -m compileall .","test":"pytest","run":"python pot.py"}`,
			},
		},
	}
	manifests := make([]*EnvironmentManifest, 0, len(fixtures))
	for _, fixture := range fixtures {
		preparer := &Preparer{
			Runtime:     &agentruntime.PurposeBuiltRuntime{Client: writingClient(fixture.files)},
			Provisioner: HostProvisioner{},
		}
		result, err := preparer.Prepare(t.Context(), fixture.request)
		if err != nil {
			t.Fatalf("%s: %v", fixture.request.Technology, err)
		}
		if result.ProposedManifest == nil {
			t.Fatalf("%s: no manifest drafted", fixture.request.Technology)
		}
		if passed, reason := fakeVerify(result.ProposedManifest); !passed {
			t.Fatalf("%s: fake verifier rejected: %s", fixture.request.Technology, reason)
		}
		manifests = append(manifests, result.ProposedManifest)
		if err := preparer.Provisioner.Destroy(t.Context(), result.Workspace); err != nil {
			t.Fatal(err)
		}
	}
	if maskRequest(preparationInstructions(fixtures[0].request), fixtures[0].request) !=
		maskRequest(preparationInstructions(fixtures[1].request), fixtures[1].request) {
		t.Fatal("preparation instructions differ by more than interpolated technology and objective")
	}
	if manifests[0].Artifact.Technology == manifests[1].Artifact.Technology {
		t.Fatal("manifests do not carry their own technologies")
	}
	tampered := *manifests[0]
	tampered.Workspace.Protected = append(append([]string{}, manifests[0].Workspace.Protected...), manifests[0].Workspace.LearnerEditable[0])
	if passed, _ := fakeVerify(&tampered); passed {
		t.Fatal("fake verifier accepted an overlapping editable/protected split")
	}
}
