package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/workflow"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

func TestWorkflowSSEReplaysOnlyEventsAfterCursor(t *testing.T) {
	service := workflow.NewService(workflow.NewInMemoryStore(), nil)
	ctx := workflowTestContext("workspace-a", "user-a")
	created, err := service.Create(ctx, workflow.CreateInput{Kind: workflow.KindMCQGeneration})
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := service.Attach(ctx, created.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := reporter.Report(ctx, "load_context", workflow.StatusRunning, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := reporter.Report(ctx, "questions_ready", workflow.StatusSucceeded, map[string]any{
		"question_count": 5,
	}, true); err != nil {
		t.Fatal(err)
	}

	handler := NewWorkflowHandler(service)
	request := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	request.SetPathValue("id", created.Operation.ID)
	request.Header.Set("Last-Event-ID", "5")
	recorder := httptest.NewRecorder()
	handler.Events(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if strings.Contains(body, "id: 5\n") {
		t.Fatalf("cursor event was replayed: %s", body)
	}
	if !strings.Contains(body, "id: 6\n") || !strings.Contains(body, "id: 7\n") {
		t.Fatalf("missing replayed events: %s", body)
	}
	if strings.Index(body, "id: 6\n") > strings.Index(body, "id: 7\n") {
		t.Fatalf("events reordered: %s", body)
	}
}

func TestWorkflowSSERejectsCrossScopeBeforeOpeningStream(t *testing.T) {
	service := workflow.NewService(workflow.NewInMemoryStore(), nil)
	owner := workflowTestContext("workspace-a", "user-a")
	created, err := service.Create(owner, workflow.CreateInput{Kind: workflow.KindMCQGeneration})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewWorkflowHandler(service)
	request := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(
		workflowTestContext("workspace-b", "user-a"),
	)
	request.SetPathValue("id", created.Operation.ID)
	recorder := httptest.NewRecorder()
	handler.Events(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatal("cross-scope request opened an event stream")
	}
}

func TestWorkflowSSEDisconnectReturns(t *testing.T) {
	service := workflow.NewService(workflow.NewInMemoryStore(), nil)
	ctx, cancel := context.WithCancel(workflowTestContext("workspace-a", "user-a"))
	created, err := service.Create(ctx, workflow.CreateInput{Kind: workflow.KindMCQGeneration})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewWorkflowHandler(service)
	handler.pollInterval = time.Millisecond
	request := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	request.SetPathValue("id", created.Operation.ID)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.Events(recorder, request)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not return after disconnect")
	}
}

func TestWorkflowCreateReturnsSeededEnvelope(t *testing.T) {
	service := workflow.NewService(workflow.NewInMemoryStore(), nil)
	handler := NewWorkflowHandler(service)
	request := httptest.NewRequest(
		http.MethodPost,
		"/workflow-operations",
		strings.NewReader(`{"kind":"memory_reflection"}`),
	).WithContext(workflowTestContext("workspace-a", "user-a"))
	recorder := httptest.NewRecorder()
	handler.Create(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data workflow.CreateResult `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Events) != 5 || body.Data.Events[0].StepID != "load_evidence" {
		t.Fatalf("unexpected create response: %#v", body.Data)
	}
}

func workflowTestContext(workspaceID, userID string) context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID: userID, DefaultWorkspaceID: workspaceID, WorkspaceIDs: []string{workspaceID},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: workspaceID})
}
