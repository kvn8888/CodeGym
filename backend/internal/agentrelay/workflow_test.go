package agentrelay

import (
	"errors"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/environment"
	"github.com/kvn8888/codegym/backend/internal/workflow"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

func TestWorkflowIssuesAndRevokesRelayToken(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	workflowService := workflow.NewService(workflow.NewInMemoryStore(), func() time.Time { return now })
	relayStore := NewInMemoryStore()
	relayService, err := NewService(relayStore, ServiceConfig{
		Environment: environment.Dev,
		TokenSecret: testSecret, TokenTTL: 15 * time.Minute,
		DefaultMaxTotalTokens: 100_000, DefaultMaxCostUSDMicros: 5_000_000,
		DefaultMaxWallClock: 10 * time.Minute,
		Clock:               func() time.Time { return now }, OperationChecker: workflowService,
	})
	if err != nil {
		t.Fatal(err)
	}
	workflowService.WithOperationTokens(relayService)
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{
		UserID: "user-a", DefaultWorkspaceID: "ws-a", WorkspaceIDs: []string{"ws-a"},
	})
	ctx = workspace.WithScope(ctx, workspace.Scope{WorkspaceID: "ws-a"})

	created, err := workflowService.Create(ctx, workflow.CreateInput{Kind: workflow.KindMemoryReflection})
	if err != nil {
		t.Fatal(err)
	}
	if created.Relay == nil || created.Relay.Token == "" || created.Relay.Deadline != created.Operation.CreatedAt.Add(10*time.Minute) {
		t.Fatalf("workflow relay access = %#v", created.Relay)
	}
	if _, err := relayService.Authenticate(ctx, created.Relay.Token); err != nil {
		t.Fatalf("issued workflow token: %v", err)
	}
	reporter, err := workflowService.Attach(ctx, created.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := reporter.Report(ctx, "memory_ready", workflow.StatusSucceeded, nil, true); err != nil {
		t.Fatal(err)
	}
	budget, err := relayStore.Get(ctx, "ws-a", "user-a", created.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if budget.RevokedAt == nil {
		t.Fatalf("terminal workflow did not revoke relay budget: %#v", budget)
	}
	if _, err := relayService.Authenticate(ctx, created.Relay.Token); !errors.Is(err, ErrRevokedToken) {
		t.Fatalf("terminal workflow token error = %v, want ErrRevokedToken", err)
	}
}
