package envintegration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
)

// TokenIssuer issues short-lived operation tokens for demo agent runs. It
// mirrors the relay service's issuance shape without importing server
// wiring, so tests and the demo server both satisfy it.
type TokenIssuer interface {
	IssueToken(ctx context.Context, operationID, workspaceID, userID string) (token string, err error)
}

// DemoAgentRuntime runs the primary adapter with a fresh operation token per
// run. Long builds outlive any single startup token, so minting per run
// keeps credentials short-lived while the relay budgets bound total spend.
type DemoAgentRuntime struct {
	Issue      TokenIssuer
	BinaryPath string
	Template   string
	RelayURL   string
	Model      string
	Owner      string
}

// Name identifies the underlying adapter.
func (r *DemoAgentRuntime) Name() string { return "opencode" }

// Run mints a run token and delegates to a fresh primary adapter. The token
// operation ID carries the demo prefix the env-aware checker recognizes.
func (r *DemoAgentRuntime) Run(ctx context.Context, task agentruntime.TaskSpec) (agentruntime.RunResult, error) {
	if r == nil || r.Issue == nil {
		return agentruntime.RunResult{}, fmt.Errorf("envintegration: token issuer is required")
	}
	if strings.TrimSpace(r.BinaryPath) == "" || strings.TrimSpace(r.RelayURL) == "" || strings.TrimSpace(r.Model) == "" {
		return agentruntime.RunResult{}, fmt.Errorf("envintegration: demo agent runtime is not configured")
	}
	token, err := r.Issue.IssueToken(ctx, fmt.Sprintf("env-demo-%d", time.Now().UnixNano()), "demo-workspace", r.Owner)
	if err != nil {
		return agentruntime.RunResult{}, fmt.Errorf("envintegration: issue demo run token: %w", err)
	}
	adapter, err := agentruntime.NewOpenCodeRuntime(agentruntime.OpenCodeConfig{
		BinaryPath: binaryPathOrDefault(r.BinaryPath), RuntimeTemplate: r.Template,
		RelayBaseURL: r.RelayURL, RelayToken: token, Model: r.Model,
	})
	if err != nil {
		return agentruntime.RunResult{}, err
	}
	return adapter.Run(ctx, task)
}

func binaryPathOrDefault(path string) string { return strings.TrimSpace(path) }

// EnvOperationChecker authorizes demo-prefixed operation tokens while
// delegating every other operation to the wrapped checker. Demo tokens are
// short-lived with relay-budget backstops; the prefix only routes the
// liveness check, never grants scope beyond what the relay budgets allow.
type EnvOperationChecker struct {
	Fallback agentrelayChecker
}

// agentrelayChecker mirrors the relay operation-checker shape without
// importing server wiring.
type agentrelayChecker interface {
	OperationActive(ctx context.Context, workspaceID, userID, operationID string) (bool, error)
}

// OperationActive allows live demo operations and defers the rest.
func (c *EnvOperationChecker) OperationActive(ctx context.Context, workspaceID, userID, operationID string) (bool, error) {
	if strings.HasPrefix(operationID, "env-demo-") {
		return true, nil
	}
	if c == nil || c.Fallback == nil {
		return false, nil
	}
	return c.Fallback.OperationActive(ctx, workspaceID, userID, operationID)
}
