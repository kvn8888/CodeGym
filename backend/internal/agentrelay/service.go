package agentrelay

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environment"
)

const tokenVersion = 2

type Clock func() time.Time

// OperationChecker supplies current workflow truth. Relay authorization fails
// closed if the operation does not exist or is terminal.
type OperationChecker interface {
	OperationActive(ctx context.Context, workspaceID, userID, operationID string) (bool, error)
}

type ServiceConfig struct {
	Environment             environment.Name
	TokenSecret             string
	TokenTTL                time.Duration
	DefaultMaxTotalTokens   int64
	DefaultMaxCostUSDMicros int64
	DefaultMaxWallClock     time.Duration
	Clock                   Clock
	OperationChecker        OperationChecker
}

type Service struct {
	store                   Store
	environment             environment.Name
	secret                  []byte
	tokenTTL                time.Duration
	defaultMaxTotalTokens   int64
	defaultMaxCostUSDMicros int64
	defaultMaxWallClock     time.Duration
	now                     Clock
	operations              OperationChecker
}

func NewService(store Store, cfg ServiceConfig) (*Service, error) {
	if store == nil {
		return nil, errors.New("agentrelay: store is required")
	}
	if !cfg.Environment.Valid() {
		return nil, errors.New("agentrelay: environment must be dev, stg, or prd")
	}
	secret := strings.TrimSpace(cfg.TokenSecret)
	if len(secret) < 32 {
		return nil, errors.New("agentrelay: token secret must be at least 32 bytes")
	}
	if cfg.TokenTTL <= 0 {
		return nil, errors.New("agentrelay: token TTL must be positive")
	}
	if cfg.DefaultMaxTotalTokens <= 0 || cfg.DefaultMaxCostUSDMicros <= 0 {
		return nil, errors.New("agentrelay: default token and cost ceilings must be positive")
	}
	if cfg.DefaultMaxWallClock <= 0 {
		return nil, errors.New("agentrelay: default wall-clock ceiling must be positive")
	}
	if cfg.OperationChecker == nil {
		return nil, errors.New("agentrelay: operation checker is required")
	}
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}
	return &Service{
		store:                   store,
		environment:             cfg.Environment,
		secret:                  []byte(secret),
		tokenTTL:                cfg.TokenTTL,
		defaultMaxTotalTokens:   cfg.DefaultMaxTotalTokens,
		defaultMaxCostUSDMicros: cfg.DefaultMaxCostUSDMicros,
		defaultMaxWallClock:     cfg.DefaultMaxWallClock,
		now:                     now,
		operations:              cfg.OperationChecker,
	}, nil
}

// Issue creates the server-owned budget row and signs a token scoped to exactly
// one operation, workspace, and user.
func (s *Service) Issue(ctx context.Context, input IssueInput) (IssueResult, error) {
	input.OperationID = strings.TrimSpace(input.OperationID)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.UserID = strings.TrimSpace(input.UserID)
	if input.OperationID == "" || input.WorkspaceID == "" || input.UserID == "" {
		return IssueResult{}, errors.New("agentrelay: operation, workspace, and user are required")
	}
	active, err := s.operations.OperationActive(ctx, input.WorkspaceID, input.UserID, input.OperationID)
	if err != nil {
		return IssueResult{}, fmt.Errorf("agentrelay: check operation before issue: %w", err)
	}
	if !active {
		return IssueResult{}, ErrOperationTerminal
	}

	now := s.now().UTC()
	deadline := input.Deadline.UTC()
	if input.Deadline.IsZero() {
		deadline = now.Add(s.defaultMaxWallClock)
	}
	if !deadline.After(now) {
		return IssueResult{}, errors.New("agentrelay: deadline must be in the future")
	}
	maxTokens := input.MaxTotalTokens
	if maxTokens <= 0 {
		maxTokens = s.defaultMaxTotalTokens
	}
	maxCost := input.MaxCostUSDMicros
	if maxCost <= 0 {
		maxCost = s.defaultMaxCostUSDMicros
	}
	budget := OperationBudget{
		ID: newID("relay_budget"), OperationID: input.OperationID,
		WorkspaceID: input.WorkspaceID, UserID: input.UserID,
		MaxTotalTokens: maxTokens, MaxCostUSDMicros: maxCost,
		Deadline: deadline, CreatedAt: now, UpdatedAt: now,
	}
	budget, err = s.store.Create(ctx, budget)
	if err != nil {
		return IssueResult{}, err
	}

	claims := tokenPayload{
		Version: tokenVersion, TokenID: newID("relay_token"), BudgetID: budget.ID,
		OperationID: budget.OperationID, WorkspaceID: budget.WorkspaceID,
		UserID: budget.UserID, Environment: s.environment,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(s.tokenTTL).Unix(),
	}
	token, err := s.sign(claims)
	if err != nil {
		return IssueResult{}, err
	}
	return IssueResult{Token: token, ExpiresAt: time.Unix(claims.ExpiresAt, 0).UTC(), Budget: budget}, nil
}

// Authenticate validates signature and expiry first, then checks the scoped
// store row for revocation and the workflow service for terminal state.
func (s *Service) Authenticate(ctx context.Context, token string) (Authorization, error) {
	claims, err := s.verify(token)
	if err != nil {
		return Authorization{}, err
	}
	budget, err := s.store.Get(ctx, claims.WorkspaceID, claims.UserID, claims.OperationID)
	if err != nil {
		return Authorization{}, err
	}
	if budget.ID != claims.BudgetID {
		return Authorization{}, ErrInvalidToken
	}
	if budget.RevokedAt != nil {
		return Authorization{}, ErrRevokedToken
	}
	now := s.now().UTC()
	active, err := s.operations.OperationActive(ctx, claims.WorkspaceID, claims.UserID, claims.OperationID)
	if err != nil {
		return Authorization{}, fmt.Errorf("agentrelay: check operation: %w", err)
	}
	if !active {
		_ = s.store.Revoke(ctx, claims.WorkspaceID, claims.UserID, claims.OperationID, now)
		return Authorization{}, ErrOperationTerminal
	}
	if !now.Before(budget.Deadline) {
		return Authorization{}, ErrDeadline
	}
	if budget.UsedTotalTokens >= budget.MaxTotalTokens {
		return Authorization{}, ErrTokenBudget
	}
	if budget.UsedCostUSDMicros >= budget.MaxCostUSDMicros {
		return Authorization{}, ErrCostBudget
	}
	return Authorization{Claims: claims.toClaims(), Budget: budget}, nil
}

// AddUsage atomically accumulates usage against the authenticated operation.
// The operation id is taken only from validated claims, never from the caller.
func (s *Service) AddUsage(ctx context.Context, authorization Authorization, delta UsageDelta) (OperationBudget, error) {
	delta.TotalTokens = max64(0, delta.TotalTokens)
	delta.InputTokens = max64(0, delta.InputTokens)
	delta.OutputTokens = max64(0, delta.OutputTokens)
	delta.ReasoningTokens = max64(0, delta.ReasoningTokens)
	delta.CacheReadTokens = max64(0, delta.CacheReadTokens)
	delta.CacheWriteTokens = max64(0, delta.CacheWriteTokens)
	delta.CostUSDMicros = max64(0, delta.CostUSDMicros)
	budget, err := s.store.AddUsage(
		ctx, authorization.Claims.WorkspaceID, authorization.Claims.UserID,
		authorization.Claims.OperationID, delta, s.now().UTC(),
	)
	if err != nil {
		return OperationBudget{}, err
	}
	if budget.ID != authorization.Claims.BudgetID {
		return OperationBudget{}, ErrInvalidToken
	}
	return budget, nil
}

// ValidateForOperation adds an explicit operation match for internal callers
// that already know the operation they are serving.
func (s *Service) ValidateForOperation(ctx context.Context, token, operationID string) (Authorization, error) {
	authorization, err := s.Authenticate(ctx, token)
	if err != nil {
		return Authorization{}, err
	}
	if authorization.Claims.OperationID != strings.TrimSpace(operationID) {
		return Authorization{}, ErrOperationMismatch
	}
	return authorization, nil
}

func (s *Service) Revoke(ctx context.Context, workspaceID, userID, operationID string) error {
	return s.store.Revoke(
		ctx, strings.TrimSpace(workspaceID), strings.TrimSpace(userID),
		strings.TrimSpace(operationID), s.now().UTC(),
	)
}

type tokenHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

type tokenPayload struct {
	Version     int              `json:"ver"`
	TokenID     string           `json:"jti"`
	BudgetID    string           `json:"bid"`
	OperationID string           `json:"operation_id"`
	WorkspaceID string           `json:"workspace_id"`
	UserID      string           `json:"user_id"`
	Environment environment.Name `json:"environment"`
	IssuedAt    int64            `json:"iat"`
	ExpiresAt   int64            `json:"exp"`
}

func (s *Service) sign(payload tokenPayload) (string, error) {
	headerJSON, err := json.Marshal(tokenHeader{Algorithm: "HS256", Type: "JWT"})
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := encodedHeader + "." + encodedPayload
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature, nil
}

func (s *Service) verify(token string) (tokenPayload, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return tokenPayload{}, ErrInvalidToken
	}
	signingInput := parts[0] + "." + parts[1]
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return tokenPayload{}, ErrInvalidToken
	}
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(signingInput))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return tokenPayload{}, ErrInvalidToken
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return tokenPayload{}, ErrInvalidToken
	}
	var header tokenHeader
	if json.Unmarshal(headerBytes, &header) != nil || header.Algorithm != "HS256" || header.Type != "JWT" {
		return tokenPayload{}, ErrInvalidToken
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenPayload{}, ErrInvalidToken
	}
	var payload tokenPayload
	if json.Unmarshal(payloadBytes, &payload) != nil || payload.Version != tokenVersion ||
		payload.TokenID == "" || payload.BudgetID == "" || payload.OperationID == "" ||
		payload.WorkspaceID == "" || payload.UserID == "" || !payload.Environment.Valid() ||
		payload.IssuedAt <= 0 || payload.ExpiresAt <= 0 {
		return tokenPayload{}, ErrInvalidToken
	}
	if payload.Environment != s.environment {
		return tokenPayload{}, ErrEnvironmentMismatch
	}
	now := s.now().UTC()
	if !now.Before(time.Unix(payload.ExpiresAt, 0)) {
		return tokenPayload{}, ErrExpiredToken
	}
	if time.Unix(payload.IssuedAt, 0).After(now.Add(time.Minute)) {
		return tokenPayload{}, ErrInvalidToken
	}
	return payload, nil
}

func (p tokenPayload) toClaims() Claims {
	return Claims{
		TokenID: p.TokenID, BudgetID: p.BudgetID, OperationID: p.OperationID,
		WorkspaceID: p.WorkspaceID, UserID: p.UserID, Environment: p.Environment,
		IssuedAt: time.Unix(p.IssuedAt, 0).UTC(), ExpiresAt: time.Unix(p.ExpiresAt, 0).UTC(),
	}
}

func newID(prefix string) string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
