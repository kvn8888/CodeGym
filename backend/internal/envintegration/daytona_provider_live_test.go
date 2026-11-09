package envintegration

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/types"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
)

// TestDaytonaProviderLive proves the real SDK adapter leases, executes, and
// releases a sandbox. It never runs unasked: set CODEGYM_RUN_DAYTONA_LIVE=1
// under Doppler. It costs one short-lived ephemeral sandbox.
func TestDaytonaProviderLive(t *testing.T) {
	if os.Getenv("CODEGYM_RUN_DAYTONA_LIVE") != "1" {
		t.Skip("set CODEGYM_RUN_DAYTONA_LIVE=1 under Doppler to lease a real sandbox")
	}
	apiKey := strings.TrimSpace(os.Getenv("DAYTONA_API_KEY"))
	if apiKey == "" {
		t.Skip("set DAYTONA_API_KEY via Doppler to lease a real sandbox")
	}
	snapshot := strings.TrimSpace(os.Getenv("CODEGYM_PROBE_SNAPSHOT"))
	if snapshot == "" {
		snapshot = "daytona-small"
	}
	client, err := NewDaytonaClient(apiKey, os.Getenv("DAYTONA_API_URL"))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	sandbox, err := client.Create(t.Context(), types.SnapshotParams{
		Snapshot: snapshot,
		SandboxBaseParams: types.SandboxBaseParams{
			Labels:          map[string]string{"codegym": "live-test"},
			NetworkBlockAll: true,
			Ephemeral:       true,
		},
	})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	defer func() {
		if err := sandbox.Delete(t.Context()); err != nil {
			t.Errorf("delete sandbox: %v", err)
		}
	}()

	exitCode, output, err := sandbox.ExecuteCommand(t.Context(), "echo codegym-live-$((6*7))", 30*time.Second)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d (%s)", exitCode, output)
	}
	if !strings.Contains(output, "codegym-live-42") {
		t.Fatalf("unexpected output: %q", output)
	}

	// The snapshot provider must apply the isolated network posture.
	provider := &DaytonaSnapshotProvider{Client: client, CommandTimeout: 30 * time.Second}
	boot, err := provider.BootFresh(t.Context(), snapshot, environmentregistry.NetworkPolicyIsolated)
	if err != nil {
		t.Fatalf("boot fresh: %v", err)
	}
	defer func() {
		if err := boot.Close(t.Context()); err != nil {
			t.Errorf("close boot: %v", err)
		}
	}()
	result, err := boot.Run(t.Context(), "curl -sS -m 8 https://registry.npmjs.org/ >/dev/null && echo REACHED || echo BLOCKED")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(result.Output, "REACHED") {
		t.Fatalf("isolated boot reached the network: %s", result.Output)
	}
}
