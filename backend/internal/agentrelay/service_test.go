package agentrelay

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environment"
)

const testSecret = "relay-test-secret-at-least-thirty-two-bytes-long"

type operationChecker struct {
	active map[string]bool
}

func (c *operationChecker) OperationActive(_ context.Context, workspaceID, userID, operationID string) (bool, error) {
	return c.active[workspaceID+"\x00"+userID+"\x00"+operationID], nil
}

func TestTokenIssueValidateRevokeAndScope(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	checker := &operationChecker{active: map[string]bool{
		"ws-a\x00user-a\x00op-a": true,
		"ws-a\x00user-a\x00op-b": true,
	}}
	store := NewInMemoryStore()
	service := newTestService(t, store, checker, &now)

	issued, err := service.Issue(context.Background(), IssueInput{
		OperationID: "op-a", WorkspaceID: "ws-a", UserID: "user-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if issued.Token == "" || issued.Budget.MaxTotalTokens != 10_000 || issued.Budget.MaxCostUSDMicros != 2_000_000 {
		t.Fatalf("unexpected issue result: %#v", issued)
	}
	parts := strings.Split(issued.Token, ".")
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode signed payload: %v", err)
	}
	var signedPayload struct {
		Environment string `json:"environment"`
	}
	if err := json.Unmarshal(payloadJSON, &signedPayload); err != nil || signedPayload.Environment != "stg" {
		t.Fatalf("signed payload environment = %q err=%v", signedPayload.Environment, err)
	}

	authorization, err := service.Authenticate(context.Background(), issued.Token)
	if err != nil {
		t.Fatalf("valid token: %v", err)
	}
	if authorization.Claims.OperationID != "op-a" || authorization.Claims.WorkspaceID != "ws-a" || authorization.Claims.UserID != "user-a" {
		t.Fatalf("unexpected token scope: %#v", authorization.Claims)
	}
	if authorization.Claims.Environment != environment.Stg {
		t.Fatalf("token environment = %q, want stg", authorization.Claims.Environment)
	}

	if _, err := service.ValidateForOperation(context.Background(), issued.Token, "op-b"); !errors.Is(err, ErrOperationMismatch) {
		t.Fatalf("wrong-operation error = %v, want ErrOperationMismatch", err)
	}
	if _, err := store.Get(context.Background(), "ws-b", "user-a", "op-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace budget read = %v, want ErrNotFound", err)
	}
	if _, err := store.Get(context.Background(), "ws-a", "user-b", "op-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user budget read = %v, want ErrNotFound", err)
	}

	if err := service.Revoke(context.Background(), "ws-a", "user-a", "op-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), issued.Token); !errors.Is(err, ErrRevokedToken) {
		t.Fatalf("revoked token error = %v, want ErrRevokedToken", err)
	}
}

func TestTokenRejectsAnotherEnvironmentWithSharedSecret(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	checker := &operationChecker{active: map[string]bool{"ws-a\x00user-a\x00op-a": true}}
	store := NewInMemoryStore()
	staging := newTestServiceForEnvironment(t, store, checker, &now, environment.Stg)
	production := newTestServiceForEnvironment(t, store, checker, &now, environment.Prd)

	issued, err := staging.Issue(context.Background(), IssueInput{
		OperationID: "op-a", WorkspaceID: "ws-a", UserID: "user-a",
	})
	if err != nil {
		t.Fatalf("issue staging token: %v", err)
	}
	if _, err := staging.Authenticate(context.Background(), issued.Token); err != nil {
		t.Fatalf("matching-environment token: %v", err)
	}
	if _, err := production.Authenticate(context.Background(), issued.Token); !errors.Is(err, ErrEnvironmentMismatch) {
		t.Fatalf("production validation error = %v, want ErrEnvironmentMismatch", err)
	}
}

func TestTokenRejectsExpiryBadSignatureAndTerminalOperation(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	checker := &operationChecker{active: map[string]bool{"ws-a\x00user-a\x00op-a": true}}
	service := newTestService(t, NewInMemoryStore(), checker, &now)
	issued, err := service.Issue(context.Background(), IssueInput{
		OperationID: "op-a", WorkspaceID: "ws-a", UserID: "user-a",
	})
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.Split(issued.Token, ".")
	replacement := byte('A')
	if parts[2][0] == replacement {
		replacement = 'B'
	}
	parts[2] = string(replacement) + parts[2][1:]
	if _, err := service.Authenticate(context.Background(), strings.Join(parts, ".")); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("bad-signature error = %v, want ErrInvalidToken", err)
	}

	checker.active["ws-a\x00user-a\x00op-a"] = false
	if _, err := service.Authenticate(context.Background(), issued.Token); !errors.Is(err, ErrOperationTerminal) {
		t.Fatalf("terminal-operation error = %v, want ErrOperationTerminal", err)
	}

	checker.active["ws-a\x00user-a\x00op-a"] = true
	now = issued.ExpiresAt
	if _, err := service.Authenticate(context.Background(), issued.Token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expired-token error = %v, want ErrExpiredToken", err)
	}
}

func newTestService(t *testing.T, store Store, checker OperationChecker, now *time.Time) *Service {
	return newTestServiceForEnvironment(t, store, checker, now, environment.Stg)
}

func newTestServiceForEnvironment(
	t *testing.T,
	store Store,
	checker OperationChecker,
	now *time.Time,
	codegymEnvironment environment.Name,
) *Service {
	t.Helper()
	service, err := NewService(store, ServiceConfig{
		Environment: codegymEnvironment,
		TokenSecret: testSecret, TokenTTL: 5 * time.Minute,
		DefaultMaxTotalTokens: 10_000, DefaultMaxCostUSDMicros: 2_000_000,
		DefaultMaxWallClock: 10 * time.Minute,
		Clock:               func() time.Time { return *now }, OperationChecker: checker,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
