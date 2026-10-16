package envintegration

import (
	"testing"

	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
)

func TestLocalSnapshotProviderBootsAndRuns(t *testing.T) {
	dirs := &SnapshotDirectories{}
	root := t.TempDir()
	reference := dirs.Register(root)
	provider := &LocalSnapshotProvider{Roots: dirs.Resolve, Runner: &LocalCommandRunner{}}
	boot, err := provider.BootFresh(t.Context(), reference, environmentregistry.NetworkPolicyIsolated)
	if err != nil {
		t.Fatal(err)
	}
	result, err := boot.Run(t.Context(), "true")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	failed, err := boot.Run(t.Context(), "false")
	if err != nil {
		t.Fatal(err)
	}
	if failed.ExitCode == 0 {
		t.Fatalf("result = %#v, want nonzero", failed)
	}
	workspace, err := boot.NewWorkspace(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if workspace.ID == "" {
		t.Fatal("empty workspace identity")
	}
	if err := boot.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.BootFresh(t.Context(), "local-missing", environmentregistry.NetworkPolicyIsolated); err == nil {
		t.Fatal("expected unknown reference to fail")
	}
}
