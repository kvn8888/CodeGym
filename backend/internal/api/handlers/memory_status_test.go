package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/session"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

func memoryStatusTestContext(workspaceID, userID string) context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID: userID, DefaultWorkspaceID: workspaceID, WorkspaceIDs: []string{workspaceID},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: workspaceID})
}

func TestPersistMemoryStatusSkipsStaleCompletion(t *testing.T) {
	ctx := memoryStatusTestContext("personal-kevin", "kevin")
	sessions := session.NewService(session.NewInMemoryStore(), nil)
	created, err := sessions.Create(ctx, session.CreateInput{Kind: session.KindWorkspace, ProblemID: "two-sum"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := sessions.SetMemoryUpdateStatusForSubmission(ctx, created.ID, "exec-new", "pending"); err != nil {
		t.Fatalf("seed newer pending status: %v", err)
	}
	handler := NewGenerateHandler(nil, nil, nil, false, nil, nil, sessions, nil, nil)

	if err := handler.persistMemoryStatus(ctx, created.ID, "exec-old", "failed"); err != nil {
		t.Fatalf("persist stale completion: %v", err)
	}
	stored, err := sessions.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	state := map[string]any{}
	if err := json.Unmarshal(stored.State, &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state["memory_update_status"] != "pending" || state["last_submission_id"] != "exec-new" {
		t.Fatalf("stale completion cleared newer status: %#v", state)
	}

	if err := handler.persistMemoryStatus(ctx, created.ID, "exec-new", "synced"); err != nil {
		t.Fatalf("persist current completion: %v", err)
	}
	stored, err = sessions.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	state = map[string]any{}
	if err := json.Unmarshal(stored.State, &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state["memory_update_status"] != "synced" || state["last_submission_id"] != "exec-new" {
		t.Fatalf("current completion did not persist status: %#v", state)
	}
}
