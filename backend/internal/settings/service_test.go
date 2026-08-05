package settings

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

func TestHedgeCountDefaultsWhenAbsent(t *testing.T) {
	service := NewService(NewInMemoryStore(), nil)
	if got := service.HedgeCount(settingsContext("workspace-a", "user-a")); got != DefaultHedgeCount {
		t.Fatalf("HedgeCount() = %d, want default %d", got, DefaultHedgeCount)
	}
}

func TestHedgeCountDefaultsWhenStoredValueIsCorrupt(t *testing.T) {
	store := NewInMemoryStore()
	_ = store.Upsert(context.Background(), Setting{
		WorkspaceID: "workspace-a", Key: HedgeCountKey,
		Type: ValueTypeInt, RawValue: "not-an-int",
	})
	service := NewService(store, nil)
	if got := service.HedgeCount(settingsContext("workspace-a", "user-a")); got != DefaultHedgeCount {
		t.Fatalf("HedgeCount() = %d, want default %d", got, DefaultHedgeCount)
	}
}

func TestHedgeCountClampsStoredValues(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want int
	}{
		{name: "below", raw: "-7", want: MinHedgeCount},
		{name: "above", raw: "99", want: MaxHedgeCount},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewInMemoryStore()
			_ = store.Upsert(context.Background(), Setting{
				WorkspaceID: "workspace-a", Key: HedgeCountKey,
				Type: ValueTypeInt, RawValue: test.raw,
			})
			service := NewService(store, nil)
			if got := service.HedgeCount(settingsContext("workspace-a", "user-a")); got != test.want {
				t.Fatalf("HedgeCount() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestRuntimeSettingCacheExpires(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	store := NewInMemoryStore()
	ctx := settingsContext("workspace-a", "user-a")
	_ = store.Upsert(ctx, Setting{
		WorkspaceID: "workspace-a", Key: HedgeCountKey,
		Type: ValueTypeInt, RawValue: "1",
	})
	service := NewService(store, func() time.Time { return now })
	service.cacheTTL = 10 * time.Second

	if got := service.HedgeCount(ctx); got != 1 {
		t.Fatalf("initial HedgeCount() = %d", got)
	}
	_ = store.Upsert(ctx, Setting{
		WorkspaceID: "workspace-a", Key: HedgeCountKey,
		Type: ValueTypeInt, RawValue: "3",
	})
	if got := service.HedgeCount(ctx); got != 1 {
		t.Fatalf("cached HedgeCount() = %d, want 1", got)
	}
	now = now.Add(10 * time.Second)
	if got := service.HedgeCount(ctx); got != 3 {
		t.Fatalf("expired HedgeCount() = %d, want 3", got)
	}
}

func TestRuntimeSettingStoreErrorDoesNotPropagate(t *testing.T) {
	service := NewService(errorStore{}, nil)
	if got := service.HedgeCount(settingsContext("workspace-a", "user-a")); got != DefaultHedgeCount {
		t.Fatalf("HedgeCount() = %d, want safe default %d", got, DefaultHedgeCount)
	}
}

func TestSetClampsAndScopesHedgeCount(t *testing.T) {
	store := NewInMemoryStore()
	service := NewService(store, nil)
	ctx := settingsContext("workspace-a", "user-a")
	resolved, err := service.Set(ctx, HedgeCountKey, json.RawMessage("99"))
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if resolved.Value != MaxHedgeCount {
		t.Fatalf("Set value = %#v, want %d", resolved.Value, MaxHedgeCount)
	}
	stored, err := store.Get(ctx, "workspace-a", HedgeCountKey)
	if err != nil {
		t.Fatalf("Get stored: %v", err)
	}
	if stored.RawValue != "3" || stored.UpdatedByUserID != "user-a" {
		t.Fatalf("stored = %#v", stored)
	}
	if got := service.HedgeCount(settingsContext("workspace-b", "user-b")); got != DefaultHedgeCount {
		t.Fatalf("cross-workspace HedgeCount() = %d, want default", got)
	}
}

func TestBoolReadsTypedValuesAndDefaultsCorruptRows(t *testing.T) {
	store := NewInMemoryStore()
	ctx := settingsContext("workspace-a", "user-a")
	_ = store.Upsert(ctx, Setting{WorkspaceID: "workspace-a", Key: "feature.enabled", Type: ValueTypeBool, RawValue: "true"})
	service := NewService(store, nil)
	if !service.Bool(ctx, "feature.enabled", false) {
		t.Fatal("Bool() = false, want true")
	}

	corrupt := NewInMemoryStore()
	_ = corrupt.Upsert(ctx, Setting{WorkspaceID: "workspace-a", Key: "feature.enabled", Type: ValueTypeBool, RawValue: "maybe"})
	if got := NewService(corrupt, nil).Bool(ctx, "feature.enabled", false); got {
		t.Fatal("corrupt Bool() = true, want default false")
	}
}

type errorStore struct{}

func (errorStore) EnsureSchema(context.Context) error { return nil }
func (errorStore) Get(context.Context, string, string) (Setting, error) {
	return Setting{}, errors.New("database unavailable")
}
func (errorStore) Upsert(context.Context, Setting) error { return errors.New("database unavailable") }

func settingsContext(workspaceID, userID string) context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{UserID: userID})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: workspaceID})
}
