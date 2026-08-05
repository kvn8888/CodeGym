package execution

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/types"
)

type fixedHedgeCount int

func (count fixedHedgeCount) HedgeCount(context.Context) int { return int(count) }

type scriptedDaytonaClient struct {
	mu            sync.Mutex
	createCalls   int
	activeCreates int
	maxConcurrent int
	params        []types.SnapshotParams
	create        func(context.Context, int, types.SnapshotParams) (daytonaRunnerSandbox, error)
}

func (c *scriptedDaytonaClient) Create(ctx context.Context, params types.SnapshotParams) (daytonaRunnerSandbox, error) {
	c.mu.Lock()
	c.createCalls++
	call := c.createCalls
	c.activeCreates++
	if c.activeCreates > c.maxConcurrent {
		c.maxConcurrent = c.activeCreates
	}
	c.params = append(c.params, params)
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.activeCreates--
		c.mu.Unlock()
	}()
	return c.create(ctx, call, params)
}

func (c *scriptedDaytonaClient) stats() (calls, maxConcurrent int, params []types.SnapshotParams) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.createCalls, c.maxConcurrent, append([]types.SnapshotParams(nil), c.params...)
}

type hedgeTestSandbox struct {
	mu              sync.Mutex
	createFolderErr error
	executeResult   *types.ExecuteResponse
	downloads       map[string][]byte
	executeCalls    int
	deleteCalls     int
	deleteContexts  chan error
}

func newHedgeTestSandbox() *hedgeTestSandbox {
	return &hedgeTestSandbox{deleteContexts: make(chan error, 2)}
}

func (s *hedgeTestSandbox) CreateFolder(context.Context, string) error {
	return s.createFolderErr
}

func (s *hedgeTestSandbox) UploadFile(context.Context, []byte, string) error { return nil }

func (s *hedgeTestSandbox) DownloadFile(_ context.Context, path string) ([]byte, error) {
	data, ok := s.downloads[path]
	if !ok {
		return nil, errors.New("file not found")
	}
	return data, nil
}

func (s *hedgeTestSandbox) ExecuteCommand(context.Context, string, time.Duration) (*types.ExecuteResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executeCalls++
	return s.executeResult, nil
}

func (s *hedgeTestSandbox) Delete(ctx context.Context) error {
	s.mu.Lock()
	s.deleteCalls++
	s.mu.Unlock()
	s.deleteContexts <- ctx.Err()
	return nil
}

func (s *hedgeTestSandbox) counts() (execute, deleted int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.executeCalls, s.deleteCalls
}

func TestCreateSandboxHedgeOneUsesLegacyPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &scriptedDaytonaClient{create: func(ctx context.Context, _ int, _ types.SnapshotParams) (daytonaRunnerSandbox, error) {
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Errorf("legacy Create context error = %v, want canceled request context", ctx.Err())
		}
		return nil, errors.New("create unavailable")
	}}
	runner := &DaytonaRunner{
		client: client, createBackoff: time.Second,
		hedgeCountProvider: fixedHedgeCount(1),
	}

	result := make(chan error, 1)
	go func() {
		_, attempts, err := runner.createSandbox(ctx, types.SnapshotParams{})
		if attempts != 1 {
			result <- errors.New("legacy canceled path did not stop after one attempt")
			return
		}
		result <- err
	}()
	select {
	case err := <-result:
		if !errors.Is(err, ErrInfrastructureBusy) {
			t.Fatalf("legacy path error = %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("hedge_count=1 did not take the cancellation-aware legacy path")
	}
	calls, maxConcurrent, _ := client.stats()
	if calls != 1 || maxConcurrent != 1 {
		t.Fatalf("legacy creates calls=%d max_concurrent=%d, want 1 and 1", calls, maxConcurrent)
	}
}

func TestCreateSandboxHedgeTwoReturnsFirstWinnerAndDeletesLateLoser(t *testing.T) {
	fast := newHedgeTestSandbox()
	slow := newHedgeTestSandbox()
	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	client := &scriptedDaytonaClient{create: func(_ context.Context, call int, _ types.SnapshotParams) (daytonaRunnerSandbox, error) {
		if call == 1 {
			close(slowStarted)
			<-releaseSlow
			return slow, nil
		}
		<-slowStarted
		return fast, nil
	}}
	runner := &DaytonaRunner{client: client, hedgeCountProvider: fixedHedgeCount(2)}
	params := types.SnapshotParams{SandboxBaseParams: types.SandboxBaseParams{
		Labels: map[string]string{"existing": "preserved"},
	}}

	type createReturn struct {
		sandbox daytonaRunnerSandbox
		err     error
	}
	returned := make(chan createReturn, 1)
	go func() {
		sandbox, _, err := runner.createSandbox(context.Background(), params)
		returned <- createReturn{sandbox: sandbox, err: err}
	}()
	var winner daytonaRunnerSandbox
	select {
	case result := <-returned:
		if result.err != nil {
			t.Fatalf("createSandbox: %v", result.err)
		}
		winner = result.sandbox
	case <-time.After(time.Second):
		t.Fatal("fast hedge did not win while the slow create was in flight")
	}
	if _, deleted := slow.counts(); deleted != 0 {
		t.Fatal("slow loser was deleted before its create completed")
	}
	close(releaseSlow)
	assertDeletedWithLiveContext(t, slow)

	if err := winner.Delete(context.Background()); err != nil {
		t.Fatalf("delete winner: %v", err)
	}
	_, _, captured := client.stats()
	if len(captured) != 2 {
		t.Fatalf("captured params = %d, want 2", len(captured))
	}
	runID := captured[0].SandboxBaseParams.Labels["codegym-run-id"]
	createdAt := captured[0].SandboxBaseParams.Labels["codegym-created-at"]
	if runID == "" || createdAt == "" {
		t.Fatalf("missing safety labels: %#v", captured[0].SandboxBaseParams.Labels)
	}
	for _, createParams := range captured {
		labels := createParams.SandboxBaseParams.Labels
		if labels["codegym"] != "submission" || labels["existing"] != "preserved" ||
			labels["codegym-run-id"] != runID || labels["codegym-created-at"] != createdAt {
			t.Fatalf("candidate labels = %#v", labels)
		}
	}
	if _, mutated := params.SandboxBaseParams.Labels["codegym-run-id"]; mutated {
		t.Fatal("createSandbox mutated the caller's labels map")
	}
}

func TestCreateSandboxHedgeAllFailReturnsInfrastructureBusy(t *testing.T) {
	client := &scriptedDaytonaClient{create: func(context.Context, int, types.SnapshotParams) (daytonaRunnerSandbox, error) {
		return nil, errors.New("placement failed")
	}}
	runner := &DaytonaRunner{
		client: client, createBackoff: 0,
		hedgeCountProvider: fixedHedgeCount(2),
	}
	_, attempts, err := runner.createSandbox(context.Background(), types.SnapshotParams{})
	if !errors.Is(err, ErrInfrastructureBusy) {
		t.Fatalf("error = %v", err)
	}
	if calls, maxConcurrent, _ := client.stats(); calls != 4 || maxConcurrent > 2 {
		t.Fatalf("calls=%d max_concurrent=%d, want 4 calls bounded to 2 lanes", calls, maxConcurrent)
	}
	if attempts != 4 {
		t.Fatalf("attempts=%d, want 4", attempts)
	}
}

func TestDaytonaRunnerHedgeExecutesUserCodeExactlyOnce(t *testing.T) {
	resultJSON := []byte(`{"schema":1,"status":"passed","cases":[{"name":"case-1","status":"pass","duration_ms":1,"error":null}],"compile_error":null,"failure_detail":null,"exit_code":0,"signal":null,"duration_ms":4,"stdout":"ok","stderr":"","output_truncated":false}`)
	fast := newHedgeTestSandbox()
	fast.executeResult = &types.ExecuteResponse{ExitCode: 0}
	fast.downloads = map[string][]byte{resultRemotePath: resultJSON}
	slow := newHedgeTestSandbox()
	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	client := &scriptedDaytonaClient{create: func(_ context.Context, call int, _ types.SnapshotParams) (daytonaRunnerSandbox, error) {
		if call == 1 {
			close(slowStarted)
			<-releaseSlow
			return slow, nil
		}
		<-slowStarted
		return fast, nil
	}}
	runner := &DaytonaRunner{client: client, hedgeCountProvider: fixedHedgeCount(2)}

	outcome, err := runner.Run(context.Background(), pythonSpec(passingSolution, solutionTests))
	if err != nil || outcome.Result.Status != JudgeStatusPassed {
		t.Fatalf("Run outcome=%#v err=%v", outcome, err)
	}
	close(releaseSlow)
	assertDeletedWithLiveContext(t, slow)
	fastExec, fastDeleted := fast.counts()
	slowExec, slowDeleted := slow.counts()
	if fastExec != 1 || slowExec != 0 {
		t.Fatalf("execute calls fast=%d slow=%d, want exactly one total", fastExec, slowExec)
	}
	if fastDeleted != 1 || slowDeleted != 1 {
		t.Fatalf("delete calls fast=%d slow=%d, want both deleted", fastDeleted, slowDeleted)
	}
}

func TestDaytonaRunnerHedgeDeletesLoserWhenWinnerRunFails(t *testing.T) {
	fast := newHedgeTestSandbox()
	fast.createFolderErr = errors.New("winner setup failed")
	slow := newHedgeTestSandbox()
	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	client := &scriptedDaytonaClient{create: func(_ context.Context, call int, _ types.SnapshotParams) (daytonaRunnerSandbox, error) {
		if call == 1 {
			close(slowStarted)
			<-releaseSlow
			return slow, nil
		}
		<-slowStarted
		return fast, nil
	}}
	runner := &DaytonaRunner{client: client, hedgeCountProvider: fixedHedgeCount(2)}

	_, err := runner.Run(context.Background(), pythonSpec(passingSolution, solutionTests))
	if err == nil || !strings.Contains(err.Error(), "create Daytona work directory") {
		t.Fatalf("Run error = %v", err)
	}
	close(releaseSlow)
	assertDeletedWithLiveContext(t, slow)
	if _, deleted := fast.counts(); deleted != 1 {
		t.Fatalf("winner delete calls = %d, want 1", deleted)
	}
}

func TestCreateSandboxHedgeDeletesLoserAfterRequestCancellation(t *testing.T) {
	fast := newHedgeTestSandbox()
	slow := newHedgeTestSandbox()
	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	client := &scriptedDaytonaClient{create: func(ctx context.Context, call int, _ types.SnapshotParams) (daytonaRunnerSandbox, error) {
		if call == 1 {
			close(slowStarted)
			<-releaseSlow
			if ctx.Err() != nil {
				t.Errorf("slow create inherited request cancellation: %v", ctx.Err())
			}
			return slow, nil
		}
		<-slowStarted
		return fast, nil
	}}
	runner := &DaytonaRunner{client: client, hedgeCountProvider: fixedHedgeCount(2)}
	ctx, cancel := context.WithCancel(context.Background())
	winner, _, err := runner.createSandbox(ctx, types.SnapshotParams{})
	if err != nil {
		t.Fatalf("createSandbox: %v", err)
	}
	cancel()
	close(releaseSlow)
	assertDeletedWithLiveContext(t, slow)
	if err := winner.Delete(context.Background()); err != nil {
		t.Fatalf("delete winner: %v", err)
	}
}

func TestCreateSandboxClampsHedgeCountToThree(t *testing.T) {
	client := &scriptedDaytonaClient{create: func(context.Context, int, types.SnapshotParams) (daytonaRunnerSandbox, error) {
		return nil, errors.New("placement failed")
	}}
	runner := &DaytonaRunner{
		client: client, createBackoff: 0,
		hedgeCountProvider: fixedHedgeCount(99),
	}
	_, _, err := runner.createSandbox(context.Background(), types.SnapshotParams{})
	if !errors.Is(err, ErrInfrastructureBusy) {
		t.Fatalf("error = %v", err)
	}
	if calls, maxConcurrent, _ := client.stats(); calls != 6 || maxConcurrent > 3 {
		t.Fatalf("calls=%d max_concurrent=%d, want 6 calls bounded to 3 lanes", calls, maxConcurrent)
	}
}

func assertDeletedWithLiveContext(t *testing.T, sandbox *hedgeTestSandbox) {
	t.Helper()
	select {
	case contextErr := <-sandbox.deleteContexts:
		if contextErr != nil {
			t.Fatalf("delete context was already canceled: %v", contextErr)
		}
	case <-time.After(time.Second):
		t.Fatal("sandbox was not deleted")
	}
}
