package usage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

// Clock is injectable for tests.
type Clock func() time.Time

// Service records GenAI usage and exposes aggregates.
type Service struct {
	store Store
	now   Clock
}

// NewService builds a usage service.
func NewService(store Store, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: store, now: clock}
}

// Record appends a usage row for the scoped user. Failures are returned to the
// caller; generation handlers should log and continue so billing never blocks.
func (s *Service) Record(ctx context.Context, input RecordInput) (Record, error) {
	if s == nil || s.store == nil {
		return Record{}, errors.New("usage service is not configured")
	}
	identity, err := identityFromContext(ctx)
	if err != nil {
		return Record{}, err
	}

	tokensIn := input.TokensIn
	if tokensIn < 0 {
		tokensIn = 0
	}
	tokensOut := input.TokensOut
	if tokensOut < 0 {
		tokensOut = 0
	}

	record := Record{
		ID:            newID(),
		WorkspaceID:   identity.workspaceID,
		UserID:        identity.userID,
		Provider:      input.Provider,
		Model:         input.Model,
		Kind:          input.Kind,
		TokensIn:      tokensIn,
		TokensOut:     tokensOut,
		CostUSDMicros: EstimateCostMicros(input.Provider, input.Model, tokensIn, tokensOut),
		CreatedAt:     s.now().UTC(),
	}
	if err := s.store.Append(ctx, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

// RecordBestEffort logs and swallows errors so generation stays unblocked.
func (s *Service) RecordBestEffort(ctx context.Context, input RecordInput) {
	if s == nil {
		return
	}
	if _, err := s.Record(ctx, input); err != nil {
		log.Printf("genai usage record failed: %v", err)
	}
}

// Aggregate returns cost/token totals for the scoped user.
func (s *Service) Aggregate(ctx context.Context) (Aggregate, error) {
	if s == nil || s.store == nil {
		return Aggregate{}, errors.New("usage service is not configured")
	}
	identity, err := identityFromContext(ctx)
	if err != nil {
		return Aggregate{}, err
	}
	records, err := s.store.List(ctx, identity.workspaceID, identity.userID)
	if err != nil {
		return Aggregate{}, err
	}

	agg := Aggregate{
		Currency:    "USD",
		PricingAsOf: PricingAsOf,
		Rates:       append([]Rate(nil), DefaultRates...),
	}

	byProvider := map[string]*ProviderSlice{}
	byModel := map[string]*ModelSlice{}

	for _, record := range records {
		agg.CallCount++
		agg.TotalTokensIn += int64(record.TokensIn)
		agg.TotalTokensOut += int64(record.TokensOut)
		agg.TotalCostUSD += MicrosToUSD(record.CostUSDMicros)

		p := byProvider[record.Provider]
		if p == nil {
			p = &ProviderSlice{Provider: record.Provider}
			byProvider[record.Provider] = p
		}
		p.TokensIn += int64(record.TokensIn)
		p.TokensOut += int64(record.TokensOut)
		p.CostUSD += MicrosToUSD(record.CostUSDMicros)
		p.CallCount++

		key := record.Provider + "\x00" + record.Model
		m := byModel[key]
		if m == nil {
			m = &ModelSlice{Provider: record.Provider, Model: record.Model}
			byModel[key] = m
		}
		m.TokensIn += int64(record.TokensIn)
		m.TokensOut += int64(record.TokensOut)
		m.CostUSD += MicrosToUSD(record.CostUSDMicros)
		m.CallCount++
	}

	// Round money to micros for stable JSON.
	agg.TotalCostUSD = roundUSD(agg.TotalCostUSD)
	for _, p := range byProvider {
		p.CostUSD = roundUSD(p.CostUSD)
		agg.ByProvider = append(agg.ByProvider, *p)
	}
	for _, m := range byModel {
		m.CostUSD = roundUSD(m.CostUSD)
		agg.ByModel = append(agg.ByModel, *m)
	}
	return agg, nil
}

func roundUSD(v float64) float64 {
	return float64(int64(v*float64(MicrosPerUSD)+0.5)) / float64(MicrosPerUSD)
}

type identity struct {
	workspaceID string
	userID      string
}

func identityFromContext(ctx context.Context) (identity, error) {
	principal, ok := auth.PrincipalFromContext(ctx)
	if !ok || principal.UserID == "" {
		return identity{}, errors.New("authenticated principal is required")
	}
	scope, ok := workspace.ScopeFromContext(ctx)
	if !ok || scope.WorkspaceID == "" {
		return identity{}, errors.New("workspace scope is required")
	}
	return identity{workspaceID: scope.WorkspaceID, userID: principal.UserID}, nil
}

func newID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("usage-%d", time.Now().UnixNano())
	}
	return "usage-" + hex.EncodeToString(buf[:])
}
