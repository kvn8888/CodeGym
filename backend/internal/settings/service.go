package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

const defaultCacheTTL = 10 * time.Second

type Clock func() time.Time

type cacheEntry struct {
	setting   Setting
	found     bool
	expiresAt time.Time
}

// Service resolves typed runtime settings with compiled defaults. Reads are
// cached briefly and deliberately fail open to the safe default so settings
// storage can never block a submission.
type Service struct {
	store    Store
	now      Clock
	cacheTTL time.Duration

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

func NewService(store Store, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{
		store: store, now: clock, cacheTTL: defaultCacheTTL,
		cache: make(map[string]cacheEntry),
	}
}

// HedgeCount returns the request-time workspace setting, always clamped to
// 1..3. Any missing scope, missing row, corrupt row, or store error returns 1.
func (s *Service) HedgeCount(ctx context.Context) int {
	return s.Int(ctx, HedgeCountKey, DefaultHedgeCount, MinHedgeCount, MaxHedgeCount)
}

// Int reads a typed integer setting and falls back without returning an error.
func (s *Service) Int(ctx context.Context, key string, fallback, minimum, maximum int) int {
	setting, found := s.readBestEffort(ctx, key)
	if !found {
		return fallback
	}
	if setting.Type != ValueTypeInt {
		log.Printf("runtime setting defaulted workspace_id=%s key=%s reason=type_mismatch", setting.WorkspaceID, key)
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(setting.RawValue))
	if err != nil {
		log.Printf("runtime setting defaulted workspace_id=%s key=%s reason=invalid_int", setting.WorkspaceID, key)
		return fallback
	}
	clamped := clampInt(value, minimum, maximum)
	if clamped != value {
		log.Printf("runtime setting clamped workspace_id=%s key=%s minimum=%d maximum=%d", setting.WorkspaceID, key, minimum, maximum)
	}
	return clamped
}

// Bool reads a typed boolean setting and falls back without returning an error.
func (s *Service) Bool(ctx context.Context, key string, fallback bool) bool {
	setting, found := s.readBestEffort(ctx, key)
	if !found {
		return fallback
	}
	if setting.Type != ValueTypeBool {
		log.Printf("runtime setting defaulted workspace_id=%s key=%s reason=type_mismatch", setting.WorkspaceID, key)
		return fallback
	}
	value, err := strconv.ParseBool(strings.TrimSpace(setting.RawValue))
	if err != nil {
		log.Printf("runtime setting defaulted workspace_id=%s key=%s reason=invalid_bool", setting.WorkspaceID, key)
		return fallback
	}
	return value
}

// Get resolves a registered operator setting. Store and parse failures still
// return the compiled default; only an unknown key is an error.
func (s *Service) Get(ctx context.Context, key string) (ResolvedSetting, error) {
	definition, ok := definitions[key]
	if !ok {
		return ResolvedSetting{}, fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	setting, found := s.readBestEffort(ctx, key)
	resolved := resolve(definition, setting, found)
	if found && resolved.Source == "default" {
		log.Printf("runtime setting defaulted workspace_id=%s key=%s reason=corrupt_row", setting.WorkspaceID, key)
	}
	return resolved, nil
}

// Set validates a registered value, clamps bounded integers, persists it in
// the caller's workspace, and refreshes this process's cache immediately.
func (s *Service) Set(ctx context.Context, key string, raw json.RawMessage) (ResolvedSetting, error) {
	if s == nil || s.store == nil {
		return ResolvedSetting{}, errors.New("runtime settings service is not configured")
	}
	definition, ok := definitions[key]
	if !ok {
		return ResolvedSetting{}, fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	identity, err := identityFromContext(ctx)
	if err != nil {
		return ResolvedSetting{}, err
	}
	value, canonical, err := validateValue(definition, raw)
	if err != nil {
		return ResolvedSetting{}, err
	}
	setting := Setting{
		WorkspaceID:     identity.workspaceID,
		Key:             key,
		Type:            definition.valueType,
		RawValue:        canonical,
		UpdatedByUserID: identity.userID,
		UpdatedAt:       s.now().UTC(),
	}
	if err := s.store.Upsert(ctx, setting); err != nil {
		return ResolvedSetting{}, err
	}
	s.putCache(setting, true)
	updatedAt := setting.UpdatedAt
	return ResolvedSetting{
		Key: key, Type: definition.valueType, Value: value,
		Default: definition.defaultValue, Source: "stored", UpdatedAt: &updatedAt,
	}, nil
}

func (s *Service) readBestEffort(ctx context.Context, key string) (Setting, bool) {
	identity, err := identityFromContext(ctx)
	if err != nil {
		log.Printf("runtime setting defaulted key=%s reason=missing_scope err=%v", key, err)
		return Setting{}, false
	}
	if s == nil || s.store == nil {
		log.Printf("runtime setting defaulted workspace_id=%s key=%s reason=store_unconfigured", identity.workspaceID, key)
		return Setting{}, false
	}

	keyForCache := cacheKey(identity.workspaceID, key)
	now := s.now()
	s.mu.RLock()
	entry, cached := s.cache[keyForCache]
	s.mu.RUnlock()
	if cached && now.Before(entry.expiresAt) {
		return entry.setting, entry.found
	}

	setting, err := s.store.Get(ctx, identity.workspaceID, key)
	if errors.Is(err, ErrSettingNotFound) {
		log.Printf("runtime setting defaulted workspace_id=%s key=%s reason=absent", identity.workspaceID, key)
		s.putCacheForKey(keyForCache, Setting{}, false, now)
		return Setting{}, false
	}
	if err != nil {
		log.Printf("runtime setting defaulted workspace_id=%s key=%s reason=store_error err=%v", identity.workspaceID, key, err)
		s.putCacheForKey(keyForCache, Setting{}, false, now)
		return Setting{}, false
	}
	s.putCacheForKey(keyForCache, setting, true, now)
	return setting, true
}

func (s *Service) putCache(setting Setting, found bool) {
	s.putCacheForKey(cacheKey(setting.WorkspaceID, setting.Key), setting, found, s.now())
}

func (s *Service) putCacheForKey(key string, setting Setting, found bool, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = cacheEntry{setting: setting, found: found, expiresAt: now.Add(s.cacheTTL)}
}

func resolve(definition definition, setting Setting, found bool) ResolvedSetting {
	resolved := ResolvedSetting{
		Key: definition.key, Type: definition.valueType,
		Value: definition.defaultValue, Default: definition.defaultValue, Source: "default",
	}
	if !found || setting.Type != definition.valueType {
		return resolved
	}
	switch definition.valueType {
	case ValueTypeInt:
		value, err := strconv.Atoi(strings.TrimSpace(setting.RawValue))
		if err != nil {
			return resolved
		}
		resolved.Value = clampInt(value, definition.minimum, definition.maximum)
	case ValueTypeBool:
		value, err := strconv.ParseBool(strings.TrimSpace(setting.RawValue))
		if err != nil {
			return resolved
		}
		resolved.Value = value
	default:
		return resolved
	}
	resolved.Source = "stored"
	updatedAt := setting.UpdatedAt.UTC()
	resolved.UpdatedAt = &updatedAt
	return resolved
}

func validateValue(definition definition, raw json.RawMessage) (any, string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, "", fmt.Errorf("%w: value is required", ErrInvalidValue)
	}
	switch definition.valueType {
	case ValueTypeInt:
		var value int
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, "", fmt.Errorf("%w: %s must be an integer", ErrInvalidValue, definition.key)
		}
		value = clampInt(value, definition.minimum, definition.maximum)
		return value, strconv.Itoa(value), nil
	case ValueTypeBool:
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, "", fmt.Errorf("%w: %s must be a boolean", ErrInvalidValue, definition.key)
		}
		return value, strconv.FormatBool(value), nil
	default:
		return nil, "", fmt.Errorf("%w: unsupported type %s", ErrInvalidValue, definition.valueType)
	}
}

func clampInt(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

type identity struct {
	workspaceID string
	userID      string
}

func identityFromContext(ctx context.Context) (identity, error) {
	principal, ok := auth.PrincipalFromContext(ctx)
	if !ok || strings.TrimSpace(principal.UserID) == "" {
		return identity{}, errors.New("authenticated principal is required")
	}
	scope, ok := workspace.ScopeFromContext(ctx)
	if !ok || strings.TrimSpace(scope.WorkspaceID) == "" {
		return identity{}, errors.New("workspace scope is required")
	}
	return identity{workspaceID: scope.WorkspaceID, userID: principal.UserID}, nil
}
