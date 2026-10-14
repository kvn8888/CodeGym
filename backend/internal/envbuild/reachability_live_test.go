package envbuild

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/daytona"
	"github.com/daytona/clients/sdk-go/pkg/options"
	"github.com/daytona/clients/sdk-go/pkg/types"
)

// TestSandboxReachabilityProbe checks whether a generic sandbox can reach the
// registries and stand up the toolchain a cold demo needs. It gates the
// Spring Boot demonstration: reachable registries plus an installable JDK
// and Maven mean the primary cold demo is live; anything else triggers the
// explicit missing-prerequisite outcome with zero agent spend. It never runs
// unasked: set CODEGYM_RUN_REACHABILITY_PROBE=1 under Doppler. The snapshot
// under test defaults to daytona-small and is overridable without code
// changes via CODEGYM_PROBE_SNAPSHOT.
func TestSandboxReachabilityProbe(t *testing.T) {
	if os.Getenv("CODEGYM_RUN_REACHABILITY_PROBE") != "1" {
		t.Skip("set CODEGYM_RUN_REACHABILITY_PROBE=1 under Doppler to probe sandbox registry reachability")
	}
	apiKey := strings.TrimSpace(os.Getenv("DAYTONA_API_KEY"))
	if apiKey == "" {
		t.Skip("set DAYTONA_API_KEY via Doppler to probe sandbox registry reachability")
	}
	snapshot := strings.TrimSpace(os.Getenv("CODEGYM_PROBE_SNAPSHOT"))
	if snapshot == "" {
		snapshot = "daytona-small"
	}
	client, err := daytona.NewClientWithConfig(&types.DaytonaConfig{APIKey: apiKey, APIUrl: os.Getenv("DAYTONA_API_URL")})
	if err != nil {
		t.Fatalf("create Daytona client: %v", err)
	}
	ctx := t.Context()
	sandbox, err := client.Create(ctx, types.SnapshotParams{
		Snapshot: snapshot,
		SandboxBaseParams: types.SandboxBaseParams{
			Labels:          map[string]string{"codegym": "reachability-probe"},
			NetworkBlockAll: false,
			Ephemeral:       true,
		},
	})
	if err != nil {
		t.Fatalf("create probe sandbox from snapshot %q: %v", snapshot, err)
	}
	deleted := false
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if deleteErr := sandbox.DeleteWithTimeout(cleanupCtx, 30*time.Second); deleteErr != nil {
			t.Errorf("delete probe sandbox: %v", deleteErr)
			return
		}
		deleted = true
	}()
	checks := []struct {
		name    string
		command string
		timeout time.Duration
		fatal   bool
	}{
		{"maven-central", "curl -sI --max-time 20 https://repo.maven.apache.org/maven2/ | head -1", time.Minute, true},
		{"npm-registry", "curl -sI --max-time 20 https://registry.npmjs.org/express | head -1", time.Minute, true},
		{"java-present", "java -version 2>&1 | head -1", time.Minute, false},
		{"maven-present", "mvn -version 2>&1 | head -1", time.Minute, false},
	}
	for _, check := range checks {
		result, err := sandbox.Process.ExecuteCommand(ctx, check.command, options.WithExecuteTimeout(check.timeout))
		if err != nil {
			if check.fatal {
				t.Fatalf("probe %s failed: %v", check.name, err)
			}
			t.Logf("probe %s: error %v", check.name, err)
			continue
		}
		t.Logf("probe %s: exit=%d output=%q", check.name, result.ExitCode, strings.TrimSpace(result.Result))
		if check.fatal && result.ExitCode != 0 {
			t.Fatalf("probe %s unreachable: exit=%d output=%q", check.name, result.ExitCode, strings.TrimSpace(result.Result))
		}
	}
	if !deleted {
		t.Log("probe sandbox scheduled for deletion")
	}
}
