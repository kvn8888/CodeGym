package agentrelay

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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

	authorization, err := service.Authenticate(context.Background(), issued.Token)
	if err != nil {
		t.Fatalf("valid token: %v", err)
	}
	if authorization.Claims.OperationID != "op-a" || authorization.Claims.WorkspaceID != "ws-a" || authorization.Claims.UserID != "user-a" {
		t.Fatalf("unexpected token scope: %#v", authorization.Claims)
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
	t.Helper()
	service, err := NewService(store, ServiceConfig{
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
