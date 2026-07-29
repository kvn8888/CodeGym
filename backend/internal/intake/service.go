package intake

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

type MemoryReader interface {
	ProfileInputs(ctx context.Context) (memory.Profile, []memory.Event, bool, error)
}

type Service struct {
	store     Store
	memory    MemoryReader
	generator *generation.Orchestrator
	now       func() time.Time
}

func NewService(store Store, memoryReader MemoryReader, generator *generation.Orchestrator, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, memory: memoryReader, generator: generator, now: now}
}

func (s *Service) Prepare(ctx context.Context, input PrepareInput) (Record, error) {
	scope, err := scopeFromContext(ctx)
	if err != nil {
		return Record{}, err
	}
	original := strings.TrimSpace(input.Topic)
	if original == "" {
		return Record{}, errors.New("topic is required")
	}
	if len([]rune(original)) > 500 {
		return Record{}, errors.New("topic must be at most 500 characters")
	}
	normalized := NormalizeTopic(original)
	if normalized == "" {
		return Record{}, errors.New("topic must contain letters or numbers")
	}
	if !input.Restart {
		existing, findErr := s.store.FindLatestByTopic(ctx, scope.workspaceID, scope.userID, normalized)
		if findErr == nil {
			if existing.Status != StatusPending || len(existing.Questions) > 0 {
				return existing, nil
			}
			return s.retryGeneration(ctx, existing)
		}
		if !errors.Is(findErr, ErrNotFound) {
			return Record{}, findErr
		}
	}

	now := s.now().UTC()
	record := Record{
		ID: newID(), WorkspaceID: scope.workspaceID, UserID: scope.userID,
		NormalizedTopic: normalized, OriginalTopic: original,
		PracticeSeed: append(json.RawMessage(nil), input.PracticeSeed...), Questions: []generation.IntakeQuestion{},
		Answers: map[string]string{}, Status: StatusPending, CreatedAt: now, UpdatedAt: now,
	}
	var practiceSeed map[string]any
	if len(input.PracticeSeed) == 0 || json.Unmarshal(input.PracticeSeed, &practiceSeed) != nil || practiceSeed == nil {
		record.PracticeSeed = json.RawMessage(`{}`)
	}

	if !input.Restart {
		known, reason, familiarErr := s.isFamiliar(ctx, normalized)
		if familiarErr != nil {
			return Record{}, familiarErr
		}
		if known {
			record.Status = StatusSkipped
			record.SuppressionReason = reason
			record.CompletedAt = &now
			return s.store.Create(ctx, record)
		}
	}
	created, err := s.store.Create(ctx, record)
	if err != nil {
		return Record{}, err
	}
	return s.retryGeneration(ctx, created)
}

func (s *Service) retryGeneration(ctx context.Context, record Record) (Record, error) {
	if s.generator == nil {
		record.GenerationError = "Baseline questions are temporarily unavailable."
		record.UpdatedAt = s.now().UTC()
		return s.store.Update(ctx, record)
	}
	questions, _, err := generation.GenerateIntakeQuestions(ctx, s.generator, generation.IntakeQuestionSpec{
		Topic: record.NormalizedTopic, PracticeSeed: string(record.PracticeSeed),
	})
	if err != nil {
		record.GenerationError = "The model did not return a usable baseline. Retry or skip."
		record.UpdatedAt = s.now().UTC()
		return s.store.Update(ctx, record)
	}
	record.Questions = questions
	record.GenerationError = ""
	record.UpdatedAt = s.now().UTC()
	return s.store.Update(ctx, record)
}

func (s *Service) Get(ctx context.Context, id string) (Record, error) {
	scope, err := scopeFromContext(ctx)
	if err != nil {
		return Record{}, err
	}
	return s.store.Get(ctx, scope.workspaceID, scope.userID, id)
}

func (s *Service) ListPending(ctx context.Context, limit int) ([]Record, error) {
	scope, err := scopeFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 20 {
		limit = 1
	}
	return s.store.ListPending(ctx, scope.workspaceID, scope.userID, limit)
}

func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (Record, error) {
	record, err := s.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	if record.Status != StatusPending {
		return record, nil
	}
	for questionID, optionID := range input.Answers {
		if !validAnswer(record.Questions, questionID, optionID) {
			return Record{}, fmt.Errorf("invalid option %q for question %q", optionID, questionID)
		}
		record.Answers[questionID] = optionID
	}
	if input.Status != nil {
		switch *input.Status {
		case StatusPending:
		case StatusCompleted:
			if len(record.Questions) == 0 || len(record.Answers) != len(record.Questions) {
				return Record{}, errors.New("all intake questions must be answered before completion")
			}
			record.Status = StatusCompleted
		case StatusSkipped:
			record.Status = StatusSkipped
			record.SuppressionReason = "user_skipped"
		default:
			return Record{}, errors.New("status must be pending, completed, or skipped")
		}
	}
	now := s.now().UTC()
	record.UpdatedAt = now
	if record.Status != StatusPending {
		record.CompletedAt = &now
	}
	return s.store.Update(ctx, record)
}

func (s *Service) Context(ctx context.Context, id string) (*generation.PracticeIntakeContext, error) {
	if strings.TrimSpace(id) == "" {
		return nil, nil
	}
	record, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if record.Status != StatusCompleted {
		return nil, nil
	}
	answers := make([]generation.PracticeIntakeAnswer, 0, len(record.Answers))
	for _, question := range record.Questions {
		optionID, ok := record.Answers[question.ID]
		if !ok {
			continue
		}
		for _, option := range question.Options {
			if option.ID == optionID {
				answers = append(answers, generation.PracticeIntakeAnswer{
					Dimension: question.Dimension, Question: question.Text,
					OptionID: option.ID, OptionLabel: option.Label,
				})
				break
			}
		}
	}
	return &generation.PracticeIntakeContext{
		IntakeID: record.ID, NormalizedTopic: record.NormalizedTopic,
		PracticeSeed: string(record.PracticeSeed), Answers: answers,
	}, nil
}

func (s *Service) isFamiliar(ctx context.Context, topic string) (bool, string, error) {
	if s.memory == nil {
		return false, "", nil
	}
	profile, events, _, err := s.memory.ProfileInputs(ctx)
	if err != nil {
		return false, "", err
	}
	for _, skill := range profile.Skills {
		if skill.Confidence > 0 && (NormalizeTopic(skill.Label) == topic || NormalizeTopic(skill.Area) == topic) {
			return true, "profile_skill", nil
		}
	}
	for _, event := range events {
		var payload map[string]any
		if json.Unmarshal(event.Payload, &payload) != nil {
			continue
		}
		value, _ := payload["topic"].(string)
		if NormalizeTopic(value) == topic {
			return true, "demonstrated_event", nil
		}
	}
	return false, "", nil
}

var whitespace = regexp.MustCompile(`\s+`)

var topicAliases = map[string]string{
	"js": "javascript", "javascript": "javascript",
	"ts": "typescript", "typescript": "typescript",
	"golang": "go", "go language": "go", "go": "go",
	"dsa":                            "data structures and algorithms",
	"data structures algorithms":     "data structures and algorithms",
	"data structures and algorithms": "data structures and algorithms",
	"system design":                  "system design", "systems design": "system design",
	"spring boot": "spring boot", "springboot": "spring boot",
}

func NormalizeTopic(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		} else {
			builder.WriteByte(' ')
		}
	}
	normalized := whitespace.ReplaceAllString(strings.TrimSpace(builder.String()), " ")
	if alias, ok := topicAliases[normalized]; ok {
		return alias
	}
	return normalized
}

func validAnswer(questions []generation.IntakeQuestion, questionID, optionID string) bool {
	for _, question := range questions {
		if question.ID != questionID {
			continue
		}
		for _, option := range question.Options {
			if option.ID == optionID {
				return true
			}
		}
	}
	return false
}

type scope struct{ workspaceID, userID string }

func scopeFromContext(ctx context.Context) (scope, error) {
	principal, ok := auth.PrincipalFromContext(ctx)
	if !ok || principal.UserID == "" {
		return scope{}, errors.New("missing authenticated user")
	}
	workspaceScope, ok := workspace.ScopeFromContext(ctx)
	if !ok || workspaceScope.WorkspaceID == "" {
		return scope{}, errors.New("missing workspace scope")
	}
	return scope{workspaceScope.WorkspaceID, principal.UserID}, nil
}

func newID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("intake_%d", time.Now().UnixNano())
	}
	return "intake_" + hex.EncodeToString(bytes[:])
}
