package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/session"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

type testGenerator struct{}

func (testGenerator) Generate(_ context.Context, request generation.GenerateRequest) (generation.GenerateResult, error) {
	switch request.Kind {
	case generation.KindInterview:
		return generation.GenerateResult{Object: json.RawMessage(`{"question":"Tell me how you would approach this problem."}`)}, nil
	case generation.KindInterviewAssessment:
		return generation.GenerateResult{Object: json.RawMessage(`{"strengths":["Clear decomposition"],"growth_edges":["Tradeoff depth"],"topic":"Algorithms"}`)}, nil
	default:
		return generation.GenerateResult{}, errors.New("unsupported test generation kind")
	}
}

type testStreamer struct {
	calls int
	fail  bool
}

type cancellationStreamer struct{}

func (cancellationStreamer) Stream(ctx context.Context, _ generation.StreamRequest, emit func(generation.StreamDelta) error) (generation.StreamResult, error) {
	if err := emit(generation.StreamDelta{Content: "Partial"}); err != nil {
		return generation.StreamResult{}, err
	}
	<-ctx.Done()
	return generation.StreamResult{}, ctx.Err()
}

func (s *testStreamer) Stream(_ context.Context, _ generation.StreamRequest, emit func(generation.StreamDelta) error) (generation.StreamResult, error) {
	s.calls++
	if err := emit(generation.StreamDelta{Content: "Consider "}); err != nil {
		return generation.StreamResult{}, err
	}
	if s.fail {
		return generation.StreamResult{}, errors.New("interrupted")
	}
	if err := emit(generation.StreamDelta{Content: "the constraints first."}); err != nil {
		return generation.StreamResult{}, err
	}
	return generation.StreamResult{Provider: "test", Model: "test"}, nil
}

func TestInterviewLifecyclePersistsTranscriptAndCompactEvents(t *testing.T) {
	now := time.Date(2026, 7, 29, 18, 0, 0, 0, time.UTC)
	ctx := scopedContext("workspace-a", "user-a")
	memoryStore := memory.NewInMemoryStore()
	memoryService := memory.NewService(memoryStore, func() time.Time { return now })
	sessionService := session.NewService(session.NewInMemoryStore(), func() time.Time { return now })
	problemService := problems.NewService(problems.NewInMemoryStore())
	orchestrator := generation.NewOrchestrator(memoryService, testGenerator{})
	streamer := &testStreamer{}
	service := NewService(
		NewInMemoryStore(), sessionService, problemService, memoryService,
		streamer, orchestrator, generation.NewProfileSynthesizer(orchestrator, memoryService),
		func() time.Time { return now },
	)
	practiceSession, err := sessionService.Create(ctx, session.CreateInput{
		Kind: session.KindInterview, Title: "Algorithms interview",
		State: json.RawMessage(`{"topic":"Arrays"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	created, err := service.CreateOrResume(ctx, CreateThreadInput{
		Kind: KindInterview, Mode: "coding", SessionID: practiceSession.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Messages) != 1 || created.Messages[0].Role != "assistant" {
		t.Fatalf("opening messages = %#v", created.Messages)
	}

	events := []SSEEvent{}
	turn := TurnInput{ClientMessageID: "client-1", Message: "I would start with a map."}
	if err := service.Turn(ctx, created.ID, turn, func(event SSEEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := eventTypes(events); strings.Join(got, ",") != "meta,delta,delta,complete" {
		t.Fatalf("SSE events = %v", got)
	}

	// Retrying the same client id replays the completed assistant reply without
	// creating a second user message or invoking the provider again.
	replay := []SSEEvent{}
	if err := service.Turn(ctx, created.ID, turn, func(event SSEEvent) error {
		replay = append(replay, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if streamer.calls != 1 || strings.Join(eventTypes(replay), ",") != "meta,delta,complete" {
		t.Fatalf("calls=%d replay=%v", streamer.calls, eventTypes(replay))
	}
	messages, err := service.Messages(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Fatalf("messages = %#v", messages)
	}

	finished, err := service.Finish(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Thread.Status != StatusCompleted || finished.Assessment.TurnCount != 1 {
		t.Fatalf("finish = %#v", finished)
	}
	persistedSession, _ := sessionService.Get(ctx, practiceSession.ID)
	if persistedSession.Status != session.StatusCompleted {
		t.Fatalf("session status = %q", persistedSession.Status)
	}
	recorded, err := memoryService.ListEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var completed memory.Event
	for _, event := range recorded {
		if event.Type == memory.TypeInterviewCompleted {
			completed = event
		}
		if strings.Contains(string(event.Payload), "I would start") ||
			strings.Contains(string(event.Payload), "constraints first") {
			t.Fatalf("transcript leaked into memory event: %s", event.Payload)
		}
	}
	if completed.ID == "" || !strings.Contains(string(completed.Payload), `"strengths"`) {
		t.Fatalf("missing compact completion event: %#v", completed)
	}
}

func TestInterruptedTurnIsRetryableWithoutDuplicatingUserMessage(t *testing.T) {
	now := time.Date(2026, 7, 29, 18, 0, 0, 0, time.UTC)
	ctx := scopedContext("workspace-a", "user-a")
	memoryService := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	sessionService := session.NewService(session.NewInMemoryStore(), func() time.Time { return now })
	practiceSession, _ := sessionService.Create(ctx, session.CreateInput{Kind: session.KindWorkspace, Title: "Two Sum"})
	streamer := &testStreamer{fail: true}
	service := NewService(
		NewInMemoryStore(), sessionService, problems.NewService(problems.NewInMemoryStore()),
		memoryService, streamer, nil, nil, func() time.Time { return now },
	)
	thread, err := service.CreateOrResume(ctx, CreateThreadInput{Kind: KindCoach, SessionID: practiceSession.ID})
	if err != nil {
		t.Fatal(err)
	}
	input := TurnInput{ClientMessageID: "client-1", Message: "Give me a nudge"}
	if err := service.Turn(ctx, thread.ID, input, func(SSEEvent) error { return nil }); err == nil {
		t.Fatal("expected interrupted stream")
	}
	streamer.fail = false
	if err := service.Turn(ctx, thread.ID, input, func(SSEEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	messages, _ := service.Messages(ctx, thread.ID)
	userCount := 0
	interruptedCount := 0
	for _, message := range messages {
		if message.Role == "user" {
			userCount++
		}
		if message.Status == MessageInterrupted {
			interruptedCount++
		}
	}
	if userCount != 1 || interruptedCount != 1 || streamer.calls != 2 {
		t.Fatalf("users=%d interrupted=%d calls=%d messages=%#v", userCount, interruptedCount, streamer.calls, messages)
	}
}

func TestChatScopePreventsCrossWorkspaceTranscriptAccess(t *testing.T) {
	now := time.Date(2026, 7, 29, 18, 0, 0, 0, time.UTC)
	owner := scopedContext("workspace-a", "user-a")
	memoryService := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	sessionService := session.NewService(session.NewInMemoryStore(), func() time.Time { return now })
	practiceSession, _ := sessionService.Create(owner, session.CreateInput{Kind: session.KindWorkspace})
	service := NewService(
		NewInMemoryStore(), sessionService, problems.NewService(problems.NewInMemoryStore()),
		memoryService, &testStreamer{}, nil, nil, func() time.Time { return now },
	)
	thread, err := service.CreateOrResume(owner, CreateThreadInput{Kind: KindCoach, SessionID: practiceSession.ID})
	if err != nil {
		t.Fatal(err)
	}
	other := scopedContext("workspace-b", "user-b")
	if _, err := service.Messages(other, thread.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-scope access error = %v", err)
	}
}

func TestStreamingCancellationPersistsRetryableInterruptedTurn(t *testing.T) {
	now := time.Date(2026, 7, 29, 18, 0, 0, 0, time.UTC)
	ctx := scopedContext("workspace-a", "user-a")
	memoryService := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	sessionService := session.NewService(session.NewInMemoryStore(), func() time.Time { return now })
	practiceSession, _ := sessionService.Create(ctx, session.CreateInput{Kind: session.KindWorkspace})
	service := NewService(
		NewInMemoryStore(), sessionService, problems.NewService(problems.NewInMemoryStore()),
		memoryService, cancellationStreamer{}, nil, nil, func() time.Time { return now },
	)
	thread, _ := service.CreateOrResume(ctx, CreateThreadInput{Kind: KindCoach, SessionID: practiceSession.ID})
	cancelCtx, cancel := context.WithCancel(ctx)
	err := service.Turn(cancelCtx, thread.ID, TurnInput{
		ClientMessageID: "cancel-1", Message: "Help",
	}, func(event SSEEvent) error {
		if event.Type == "delta" {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	messages, _ := service.Messages(ctx, thread.ID)
	if len(messages) != 2 || messages[1].Status != MessageInterrupted ||
		messages[1].Content != "Partial" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestCoachContextIncludesWorkspaceDraftCode(t *testing.T) {
	now := time.Date(2026, 7, 29, 18, 0, 0, 0, time.UTC)
	ctx := scopedContext("workspace-a", "user-a")
	memoryService := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	sessionService := session.NewService(session.NewInMemoryStore(), func() time.Time { return now })
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(ctx); err != nil {
		t.Fatal(err)
	}
	practiceSession, err := sessionService.Create(ctx, session.CreateInput{
		Kind: session.KindWorkspace, ProblemID: "two-sum", Title: "Two Sum",
	})
	if err != nil {
		t.Fatal(err)
	}
	practiceSession, err = sessionService.UpsertFiles(ctx, practiceSession.ID, session.UpsertFilesInput{
		Files: []session.FileInput{{Path: "solution.py", Content: "def two_sum(nums, target):\n    return [0, 1]\n"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(
		NewInMemoryStore(), sessionService, problemService,
		memoryService, &testStreamer{}, nil, nil, func() time.Time { return now },
	)
	thread, err := service.CreateOrResume(ctx, CreateThreadInput{
		Kind: KindCoach, SessionID: practiceSession.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	instructions, err := service.instructions(ctx, thread.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instructions, `"source":"draft"`) ||
		!strings.Contains(instructions, "def two_sum(nums, target):") ||
		!strings.Contains(instructions, "return [0, 1]") {
		t.Fatalf("instructions missing draft code: %s", instructions)
	}
	if strings.Contains(instructions, "test_solution") ||
		strings.Contains(instructions, "CODEGYM_RESULT") ||
		strings.Contains(instructions, "Reference") {
		t.Fatalf("instructions leaked hidden tests or reference: %s", instructions)
	}
}

func TestCoachContextFallsBackToSkeletonWhenDraftEmpty(t *testing.T) {
	now := time.Date(2026, 7, 29, 18, 0, 0, 0, time.UTC)
	ctx := scopedContext("workspace-a", "user-a")
	memoryService := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	sessionService := session.NewService(session.NewInMemoryStore(), func() time.Time { return now })
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(ctx); err != nil {
		t.Fatal(err)
	}
	practiceSession, err := sessionService.Create(ctx, session.CreateInput{
		Kind: session.KindWorkspace, ProblemID: "two-sum", Title: "Two Sum",
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(
		NewInMemoryStore(), sessionService, problemService,
		memoryService, &testStreamer{}, nil, nil, func() time.Time { return now },
	)
	thread, err := service.CreateOrResume(ctx, CreateThreadInput{
		Kind: KindCoach, SessionID: practiceSession.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	instructions, err := service.instructions(ctx, thread.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instructions, `"source":"skeleton"`) ||
		!strings.Contains(instructions, "solution.py") ||
		!strings.Contains(instructions, "def two_sum") {
		t.Fatalf("instructions missing skeleton fallback: %s", instructions)
	}
}

func TestCoachContextUsesAuthoritativeMCQQuestionWithoutRawAnswer(t *testing.T) {
	now := time.Date(2026, 7, 29, 18, 0, 0, 0, time.UTC)
	ctx := scopedContext("workspace-a", "user-a")
	memoryService := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	sessionService := session.NewService(session.NewInMemoryStore(), func() time.Time { return now })
	practiceSession, _ := sessionService.Create(ctx, session.CreateInput{
		Kind: session.KindMCQ,
		State: json.RawMessage(`{
			"question_index":0,
			"response_text":"private draft answer",
			"questions":[{
				"id":"q1","type":"single_select","text":"Which traversal uses a queue?",
				"options":["DFS","BFS"],"concept":"Graph traversal","correctIndex":1
			}]
		}`),
	})
	service := NewService(
		NewInMemoryStore(), sessionService, problems.NewService(problems.NewInMemoryStore()),
		memoryService, &testStreamer{}, nil, nil, func() time.Time { return now },
	)
	thread, err := service.CreateOrResume(ctx, CreateThreadInput{
		Kind: KindCoach, SessionID: practiceSession.ID, QuestionID: "forged-question",
	})
	if err != nil {
		t.Fatal(err)
	}
	if thread.Context.QuestionID != "q1" {
		t.Fatalf("question id = %q", thread.Context.QuestionID)
	}
	instructions, err := service.instructions(ctx, thread.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instructions, "Which traversal uses a queue?") ||
		strings.Contains(instructions, "private draft answer") ||
		strings.Contains(instructions, "correctIndex") {
		t.Fatalf("instructions leaked or omitted context: %s", instructions)
	}
}

func scopedContext(workspaceID, userID string) context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID: userID, DefaultWorkspaceID: workspaceID, WorkspaceIDs: []string{workspaceID},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: workspaceID})
}

func eventTypes(events []SSEEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.Type)
	}
	return out
}
