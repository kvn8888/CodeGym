package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/practice"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

func practiceTestService(t *testing.T) *practice.Service {
	t.Helper()
	service := &practice.Service{
		Artifacts: practice.SampleArtifactSource{},
		Contents:  practice.SampleContentStore{},
		Runner:    practice.FakeSampleRunner{},
		Records:   practice.NewInMemoryRecordStore(),
		Saved:     practice.NewInMemorySavedExerciseStore(),
		Simulated: true,
	}
	return service
}

func withPracticeIdentity(req *http.Request, userID, workspaceID string) *http.Request {
	ctx := auth.WithPrincipal(req.Context(), auth.Principal{
		UserID:             userID,
		DefaultWorkspaceID: workspaceID,
		WorkspaceIDs:       []string{workspaceID},
	})
	ctx = workspace.WithScope(ctx, workspace.Scope{WorkspaceID: workspaceID})
	return req.WithContext(ctx)
}

func decodePracticeData[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var envelope struct {
		Data  T           `json:"data"`
		Error interface{} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v body=%s", err, recorder.Body.String())
	}
	if envelope.Error != nil {
		t.Fatalf("unexpected error: %v body=%s", envelope.Error, recorder.Body.String())
	}
	return envelope.Data
}

func TestPracticeOpenRequiresArtifactPin(t *testing.T) {
	handler := NewPracticeHandler(practiceTestService(t))
	req := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/sessions", strings.NewReader(`{"technology":"python"}`)), "kevin", "personal-kevin")
	recorder := httptest.NewRecorder()
	handler.Open(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want explicit pin requirement", recorder.Code, recorder.Body.String())
	}
}

func TestPracticeFullBrowserFlow(t *testing.T) {
	handler := NewPracticeHandler(practiceTestService(t))
	// Open with exact pin.
	openReq := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/sessions",
		strings.NewReader(`{"artifact_id":"greet-basics-2026-10-01","technology":"python","version":"0.1.0"}`)), "kevin", "personal-kevin")
	openRec := httptest.NewRecorder()
	handler.Open(openRec, openReq)
	if openRec.Code != http.StatusCreated {
		t.Fatalf("open status = %d body=%s", openRec.Code, openRec.Body.String())
	}
	view := decodePracticeData[practice.BrowserSession](t, openRec)
	if view.ArtifactID != "greet-basics-2026-10-01" || !view.Simulated {
		t.Fatalf("view = %+v, want exact identity plus simulated label", view)
	}
	if strings.Contains(view.Instructions, "test_greet") {
		t.Fatalf("instructions leak hidden tests")
	}
	// Forbidden path rejected.
	badReq := withPracticeIdentity(httptest.NewRequest(http.MethodPut, "/api/v1/practice/sessions/"+view.SessionID+"/files",
		strings.NewReader(`{"files":[{"path":"protected/test_greet.py","content":"x"}]}`)), "kevin", "personal-kevin")
	badReq.SetPathValue("id", view.SessionID)
	badRec := httptest.NewRecorder()
	handler.WriteFiles(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("forbidden write status = %d, want 400", badRec.Code)
	}
	// Allowed write.
	goodReq := withPracticeIdentity(httptest.NewRequest(http.MethodPut, "/api/v1/practice/sessions/"+view.SessionID+"/files",
		strings.NewReader(`{"files":[{"path":"solution.py","content":"def greet(name): return 1"}]}`)), "kevin", "personal-kevin")
	goodReq.SetPathValue("id", view.SessionID)
	goodRec := httptest.NewRecorder()
	handler.WriteFiles(goodRec, goodReq)
	if goodRec.Code != http.StatusOK {
		t.Fatalf("write status = %d body=%s", goodRec.Code, goodRec.Body.String())
	}
	// Submit returns a safe category without hidden content.
	submitReq := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/sessions/"+view.SessionID+"/submit", nil), "kevin", "personal-kevin")
	submitReq.SetPathValue("id", view.SessionID)
	submitRec := httptest.NewRecorder()
	handler.Submit(submitRec, submitReq)
	if submitRec.Code != http.StatusOK {
		t.Fatalf("submit status = %d body=%s", submitRec.Code, submitRec.Body.String())
	}
	result := decodePracticeData[practice.Result](t, submitRec)
	if result.Category != practice.CategoryTestFailure && result.Category != practice.CategoryBuildError && result.Category != practice.CategoryPassed {
		t.Fatalf("category = %q, want a safe category", result.Category)
	}
	if !result.Simulated {
		t.Fatalf("simulated = false, want explicit label")
	}
	public, _ := json.Marshal(result)
	for _, secret := range []string{"test_greet", "SAMPLE_ASSERT"} {
		if bytes.Contains(public, []byte(secret)) {
			t.Fatalf("result leaks %q: %s", secret, public)
		}
	}
	// Save returns the public receipt only: no snapshot/build references,
	// no file contents.
	saveReq := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/sessions/"+view.SessionID+"/save", nil), "kevin", "personal-kevin")
	saveReq.SetPathValue("id", view.SessionID)
	saveRec := httptest.NewRecorder()
	handler.Save(saveRec, saveReq)
	if saveRec.Code != http.StatusCreated {
		t.Fatalf("save status = %d body=%s", saveRec.Code, saveRec.Body.String())
	}
	saved := decodePracticeData[practice.SavedReceipt](t, saveRec)
	if saved.ArtifactID != "greet-basics-2026-10-01" || saved.ID == "" {
		t.Fatalf("receipt = %+v, want public identity", saved)
	}
	for _, internal := range []string{"snapshot_ref", "builder_run_id", "verifier_run_id", "sample_greet", "sample-build", "sample-verify"} {
		if strings.Contains(saveRec.Body.String(), internal) {
			t.Fatalf("save response leaks internal %q: %s", internal, saveRec.Body.String())
		}
	}
	// Cross-owner resume rejected.
	crossReq := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/saved/"+saved.ID+"/resume", nil), "alec", "personal-alec")
	crossReq.SetPathValue("id", saved.ID)
	crossRec := httptest.NewRecorder()
	handler.Resume(crossRec, crossReq)
	if crossRec.Code != http.StatusForbidden && crossRec.Code != http.StatusNotFound {
		t.Fatalf("cross-owner resume status = %d, want 403/404", crossRec.Code)
	}
	resumeReq := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/saved/"+saved.ID+"/resume", nil), "kevin", "personal-kevin")
	resumeReq.SetPathValue("id", saved.ID)
	resumeRec := httptest.NewRecorder()
	handler.Resume(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusCreated {
		t.Fatalf("resume status = %d body=%s", resumeRec.Code, resumeRec.Body.String())
	}
	resumed := decodePracticeData[practice.BrowserSession](t, resumeRec)
	if len(resumed.Files) != 1 || !strings.Contains(resumed.Files[0].Content, "def greet") {
		t.Fatalf("resumed files = %+v, want saved edits", resumed.Files)
	}
	if resumed.SessionID == view.SessionID {
		t.Fatalf("resume reused session ID")
	}
}

func TestPracticeUnavailableWithoutService(t *testing.T) {
	handler := NewPracticeHandler(nil)
	req := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/sessions", strings.NewReader(`{}`)), "kevin", "personal-kevin")
	recorder := httptest.NewRecorder()
	handler.Open(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want explicit 503", recorder.Code)
	}
}

func decodePracticeError(t *testing.T, recorder *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var envelope struct {
		Data  interface{} `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error: %v body=%s", err, recorder.Body.String())
	}
	if envelope.Error == nil {
		t.Fatalf("expected error envelope, body=%s", recorder.Body.String())
	}
	return envelope.Error.Code, envelope.Error.Message
}

// Every error surface returns a stable code with a fixed safe message:
// no snapshot/build references, file contents, hidden paths, or provider
// internals, while missing/version/build-changed outcomes stay explicit.
func TestPracticeErrorSurfacesAreSafe(t *testing.T) {
	handler := NewPracticeHandler(practiceTestService(t))
	open := func(body string) *httptest.ResponseRecorder {
		req := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/sessions", strings.NewReader(body)), "kevin", "personal-kevin")
		recorder := httptest.NewRecorder()
		handler.Open(recorder, req)
		return recorder
	}

	// Missing pin → 400 with the pin requirement, echoing nothing.
	if recorder := open(`{"technology":"python"}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("pin status = %d body=%s", recorder.Code, recorder.Body.String())
	} else {
		code, message := decodePracticeError(t, recorder)
		if code != "practice_invalid_request" || !strings.Contains(message, "artifact_id pin") {
			t.Fatalf("pin code/message = %q/%q", code, message)
		}
	}

	// Unknown artifact → 404 without echoing the requested ID.
	if recorder := open(`{"artifact_id":"other-exercise","technology":"python"}`); recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown status = %d body=%s", recorder.Code, recorder.Body.String())
	} else {
		code, _ := decodePracticeError(t, recorder)
		if code != "practice_not_found" {
			t.Fatalf("unknown code = %q", code)
		}
		if strings.Contains(recorder.Body.String(), "other-exercise") {
			t.Fatalf("unknown artifact echoes requested ID: %s", recorder.Body.String())
		}
	}

	// Version skew → 409 with the never-substituted outcome, no versions.
	if recorder := open(`{"artifact_id":"greet-basics-2026-10-01","technology":"python","version":"9.9.9"}`); recorder.Code != http.StatusConflict {
		t.Fatalf("version status = %d body=%s", recorder.Code, recorder.Body.String())
	} else {
		code, message := decodePracticeError(t, recorder)
		if code != "practice_artifact_mismatch" || !strings.Contains(message, "never substituted") {
			t.Fatalf("version code/message = %q/%q", code, message)
		}
		if strings.Contains(recorder.Body.String(), "9.9.9") {
			t.Fatalf("version mismatch echoes versions: %s", recorder.Body.String())
		}
	}

	// Valid session for file/resume error surfaces.
	openRec := open(`{"artifact_id":"greet-basics-2026-10-01","technology":"python","version":"0.1.0"}`)
	if openRec.Code != http.StatusCreated {
		t.Fatalf("open status = %d body=%s", openRec.Code, openRec.Body.String())
	}
	view := decodePracticeData[practice.BrowserSession](t, openRec)

	// Forbidden path → 400 without echoing the protected path.
	forbiddenReq := withPracticeIdentity(httptest.NewRequest(http.MethodPut, "/api/v1/practice/sessions/"+view.SessionID+"/files",
		strings.NewReader(`{"files":[{"path":"protected/test_greet.py","content":"x"}]}`)), "kevin", "personal-kevin")
	forbiddenReq.SetPathValue("id", view.SessionID)
	forbiddenRec := httptest.NewRecorder()
	handler.WriteFiles(forbiddenRec, forbiddenReq)
	if forbiddenRec.Code != http.StatusBadRequest {
		t.Fatalf("forbidden status = %d", forbiddenRec.Code)
	}
	if strings.Contains(forbiddenRec.Body.String(), "protected/test_greet.py") {
		t.Fatalf("forbidden path echoes protected path: %s", forbiddenRec.Body.String())
	}

	// Unknown session → 404.
	missingReq := withPracticeIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/practice/sessions/no-such-session", nil), "kevin", "personal-kevin")
	missingReq.SetPathValue("id", "no-such-session")
	missingRec := httptest.NewRecorder()
	handler.Get(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", missingRec.Code)
	}

	// Cross-owner → 403.
	crossReq := withPracticeIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/practice/sessions/"+view.SessionID, nil), "alec", "personal-alec")
	crossReq.SetPathValue("id", view.SessionID)
	crossRec := httptest.NewRecorder()
	handler.Get(crossRec, crossReq)
	if crossRec.Code != http.StatusForbidden {
		t.Fatalf("cross-owner status = %d body=%s", crossRec.Code, crossRec.Body.String())
	}

	// Unknown saved → 404.
	resumeReq := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/saved/no-such-saved/resume", nil), "kevin", "personal-kevin")
	resumeReq.SetPathValue("id", "no-such-saved")
	resumeRec := httptest.NewRecorder()
	handler.Resume(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusNotFound {
		t.Fatalf("saved-missing status = %d", resumeRec.Code)
	}

	// Global sweep: no error body may carry internal metadata or
	// protected content.
	bodies := []string{
		open(`{"technology":"python"}`).Body.String(),
		open(`{"artifact_id":"other-exercise","technology":"python"}`).Body.String(),
		forbiddenRec.Body.String(), missingRec.Body.String(), crossRec.Body.String(), resumeRec.Body.String(),
	}
	for _, body := range bodies {
		for _, leaked := range []string{
			"snapshot_ref", "builder_run_id", "verifier_run_id",
			"sample_greet", "sample-build", "sample-verify",
			"test_greet", "SAMPLE_ASSERT", "STRIP_WHITESPACE",
			"python3 -m py_compile", "python3 protected",
		} {
			if strings.Contains(body, leaked) {
				t.Fatalf("error body leaks %q: %s", leaked, body)
			}
		}
	}
}

// Without durable storage the service is wired with a nil saved store:
// opening and Run/Submit keep working, but Save/List/Resume refuse with
// an explicit unavailable outcome instead of reporting success a restart
// would discard.
func TestPracticeSaveUnavailableWithoutDurableStore(t *testing.T) {
	service := practiceTestService(t)
	service.Saved = nil
	handler := NewPracticeHandler(service)

	openReq := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/sessions",
		strings.NewReader(`{"artifact_id":"greet-basics-2026-10-01","technology":"python","version":"0.1.0"}`)), "kevin", "personal-kevin")
	openRec := httptest.NewRecorder()
	handler.Open(openRec, openReq)
	if openRec.Code != http.StatusCreated {
		t.Fatalf("open status = %d body=%s, want sessions to open without durable saves", openRec.Code, openRec.Body.String())
	}
	view := decodePracticeData[practice.BrowserSession](t, openRec)

	saveReq := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/sessions/"+view.SessionID+"/save", nil), "kevin", "personal-kevin")
	saveReq.SetPathValue("id", view.SessionID)
	saveRec := httptest.NewRecorder()
	handler.Save(saveRec, saveReq)
	if saveRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("save status = %d body=%s, want explicit 503, never false success", saveRec.Code, saveRec.Body.String())
	}
	if code, _ := decodePracticeError(t, saveRec); code != "practice_saved_unavailable" {
		t.Fatalf("save code = %q, want practice_saved_unavailable", code)
	}

	listReq := withPracticeIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/practice/saved", nil), "kevin", "personal-kevin")
	listRec := httptest.NewRecorder()
	handler.ListSaved(listRec, listReq)
	if listRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("list status = %d, want explicit 503", listRec.Code)
	}

	resumeReq := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/saved/whatever/resume", nil), "kevin", "personal-kevin")
	resumeReq.SetPathValue("id", "whatever")
	resumeRec := httptest.NewRecorder()
	handler.Resume(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("resume status = %d, want explicit 503", resumeRec.Code)
	}

	// Run still works: only the durability claim is gated.
	runReq := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice/sessions/"+view.SessionID+"/run", nil), "kevin", "personal-kevin")
	runReq.SetPathValue("id", view.SessionID)
	runRec := httptest.NewRecorder()
	handler.Run(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("run status = %d body=%s, want Run unaffected", runRec.Code, runRec.Body.String())
	}
}
