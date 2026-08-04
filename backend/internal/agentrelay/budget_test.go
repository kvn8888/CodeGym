package agentrelay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
	"github.com/kvn8888/codegym/backend/internal/usage"
)

func TestRelayRejectsAfterTokenCeilingAndRecordsRawPricedUsage(t *testing.T) {
	harness := newBudgetHarness(t, 40, 10_000, time.Minute)

	first := relayRequest(t, harness.handler, harness.token, `{"model":"codegym-agent","messages":[{"role":"user","content":"hi"}]}`)
	if first.Code != http.StatusOK {
		t.Fatalf("first response = %d %s", first.Code, first.Body.String())
	}
	second := relayRequest(t, harness.handler, harness.token, `{"model":"codegym-agent","messages":[{"role":"user","content":"again"}]}`)
	if second.Code != http.StatusTooManyRequests || !strings.Contains(second.Body.String(), `"code":"token_budget_exceeded"`) {
		t.Fatalf("token ceiling response = %d %s", second.Code, second.Body.String())
	}
	if *harness.upstreamCalls != 1 {
		t.Fatalf("upstream calls = %d, want 1", *harness.upstreamCalls)
	}

	budget, err := harness.relayStore.Get(t.Context(), "ws-a", "user-a", "op-a")
	if err != nil {
		t.Fatal(err)
	}
	if budget.UsedTotalTokens != 48 || budget.InputTokens != 37 || budget.OutputTokens != 11 ||
		budget.ReasoningTokens != 5 || budget.CacheReadTokens != 7 || budget.CacheWriteTokens != 3 {
		t.Fatalf("raw budget usage = %#v", budget)
	}
	expectedCost := usage.EstimateCostMicros("azure", "azure-deployment", 37, 11)
	if budget.UsedCostUSDMicros != expectedCost || expectedCost == 0 {
		t.Fatalf("budget cost = %d, expected nonzero own price %d", budget.UsedCostUSDMicros, expectedCost)
	}
	records, err := harness.usageStore.List(t.Context(), "ws-a", "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("usage records = %#v", records)
	}
	record := records[0]
	if record.OperationID != "op-a" || record.TotalTokens != 48 || record.TokensIn != 37 ||
		record.TokensOut != 11 || record.ReasoningTokens != 5 || record.CacheReadTokens != 7 ||
		record.CacheWriteTokens != 3 || record.CostUSDMicros != expectedCost || record.CostUSDMicros == 0 {
		t.Fatalf("raw usage record = %#v", record)
	}
}

func TestRelayRejectsAfterCostCeiling(t *testing.T) {
	harness := newBudgetHarness(t, 10_000, 100, time.Minute)

	first := relayRequest(t, harness.handler, harness.token, `{"model":"codegym-agent","messages":[{"role":"user","content":"hi"}]}`)
	if first.Code != http.StatusOK {
		t.Fatalf("first response = %d %s", first.Code, first.Body.String())
	}
	second := relayRequest(t, harness.handler, harness.token, `{"model":"codegym-agent","messages":[{"role":"user","content":"again"}]}`)
	if second.Code != http.StatusTooManyRequests || !strings.Contains(second.Body.String(), `"code":"cost_budget_exceeded"`) {
		t.Fatalf("cost ceiling response = %d %s", second.Code, second.Body.String())
	}
	if *harness.upstreamCalls != 1 {
		t.Fatalf("upstream calls = %d, want 1", *harness.upstreamCalls)
	}
}

func TestRelayRejectsAfterOperationDeadline(t *testing.T) {
	harness := newBudgetHarness(t, 10_000, 10_000, time.Minute)
	*harness.now = (*harness.now).Add(time.Minute)

	response := relayRequest(t, harness.handler, harness.token, `{"model":"codegym-agent","messages":[{"role":"user","content":"late"}]}`)
	if response.Code != http.StatusRequestTimeout || !strings.Contains(response.Body.String(), `"code":"operation_deadline_exceeded"`) {
		t.Fatalf("deadline response = %d %s", response.Code, response.Body.String())
	}
	if *harness.upstreamCalls != 0 {
		t.Fatalf("upstream calls after deadline = %d", *harness.upstreamCalls)
	}
}

func TestUsageAccumulationCannotAffectAnotherOperation(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	checker := &operationChecker{active: map[string]bool{
		"ws-a\x00user-a\x00op-a": true,
		"ws-a\x00user-a\x00op-b": true,
	}}
	store := NewInMemoryStore()
	service := newTestService(t, store, checker, &now)
	issuedA, err := service.Issue(t.Context(), IssueInput{OperationID: "op-a", WorkspaceID: "ws-a", UserID: "user-a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Issue(t.Context(), IssueInput{OperationID: "op-b", WorkspaceID: "ws-a", UserID: "user-a"}); err != nil {
		t.Fatal(err)
	}
	authorizationA, err := service.Authenticate(t.Context(), issuedA.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddUsage(t.Context(), authorizationA, UsageDelta{TotalTokens: 50, InputTokens: 40, OutputTokens: 10, CostUSDMicros: 25}); err != nil {
		t.Fatal(err)
	}
	budgetB, err := store.Get(t.Context(), "ws-a", "user-a", "op-b")
	if err != nil {
		t.Fatal(err)
	}
	if budgetB.UsedTotalTokens != 0 || budgetB.UsedCostUSDMicros != 0 {
		t.Fatalf("operation A token changed operation B budget: %#v", budgetB)
	}
}

type budgetHarness struct {
	handler       *HTTPHandler
	token         string
	relayStore    *InMemoryStore
	usageStore    *usage.InMemoryStore
	now           *time.Time
	upstreamCalls *int
}

func newBudgetHarness(t *testing.T, maxTokens, maxCost int64, deadlineAfter time.Duration) budgetHarness {
	t.Helper()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-budget", "object": "chat.completion", "model": "azure-deployment",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": "ok"},
			}},
			"usage": map[string]any{
				"prompt_tokens": 37, "completion_tokens": 11, "total_tokens": 48,
				"prompt_tokens_details": map[string]any{
					"cached_tokens": 7, "cache_write_tokens": 3,
				},
				"completion_tokens_details": map[string]any{"reasoning_tokens": 5},
			},
			"cost": 0,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(upstream.Close)

	checker := &operationChecker{active: map[string]bool{"ws-a\x00user-a\x00op-a": true}}
	relayStore := NewInMemoryStore()
	tokens, err := NewService(relayStore, ServiceConfig{
		TokenSecret: testSecret, TokenTTL: 5 * time.Minute,
		DefaultMaxTotalTokens: maxTokens, DefaultMaxCostUSDMicros: maxCost,
		DefaultMaxWallClock: 10 * time.Minute,
		Clock:               func() time.Time { return now }, OperationChecker: checker,
	})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := tokens.Issue(t.Context(), IssueInput{
		OperationID: "op-a", WorkspaceID: "ws-a", UserID: "user-a",
		Deadline: now.Add(deadlineAfter),
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := openaicompat.New(openaicompat.Config{
		Name: "azure", BaseURL: upstream.URL, APIKey: "provider-secret",
		Model: "azure-deployment", AuthStyle: openaicompat.AuthAzureAPIKey,
		APIVersion: "test-version", HTTPClient: http.DefaultClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	usageStore := usage.NewInMemoryStore()
	handler, err := NewHTTPHandler(tokens, adapter, usage.NewService(usageStore, func() time.Time { return now }), "codegym-agent")
	if err != nil {
		t.Fatal(err)
	}
	return budgetHarness{
		handler: handler, token: issued.Token, relayStore: relayStore,
		usageStore: usageStore, now: &now, upstreamCalls: &upstreamCalls,
	}
}
