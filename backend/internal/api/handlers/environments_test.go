package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/envintegration"
)

func instantPipeline(sessionID string, err error) envintegration.PipelineFunc {
	return func(context.Context, *envintegration.EnvironmentOperation, func(string)) (string, error) {
		return sessionID, err
	}
}

func requestEnvironment(t *testing.T, handler *EnvironmentsHandler, userID, workspaceID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := withPracticeIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/practice-environments/requests", strings.NewReader(body)), userID, workspaceID)
	recorder := httptest.NewRecorder()
	handler.Request(recorder, req)
	return recorder
}

func TestEnvironmentRequestStartsImmediately(t *testing.T) {
	handler := NewEnvironmentsHandler(&envintegration.OperationStore{Pipeline: instantPipeline("session-1", nil)})
	recorder := requestEnvironment(t, handler, "kevin", "personal-kevin", `{"technology":"demo","objective":"learn demo"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s, want accepted", recorder.Code, recorder.Body.String())
	}
	view := decodePracticeData[environmentOperationView](t, recorder)
	if view.ID == "" || view.Technology != "demo" {
		t.Fatalf("view = %#v", view)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		statusReq := withPracticeIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/practice-environments/requests/"+view.ID, nil), "kevin", "personal-kevin")
		statusReq.SetPathValue("id", view.ID)
		statusRec := httptest.NewRecorder()
		handler.Status(statusRec, statusReq)
		current := decodePracticeData[environmentOperationView](t, statusRec)
		if current.Status == "ready" {
			if current.SessionID != "session-1" {
				t.Fatalf("session = %q", current.SessionID)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation never became ready: %#v", current)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEnvironmentRequestFailureAndIsolation(t *testing.T) {
	handler := NewEnvironmentsHandler(&envintegration.OperationStore{Pipeline: instantPipeline("", errors.New("registry exploded"))})
	recorder := requestEnvironment(t, handler, "kevin", "personal-kevin", `{"technology":"demo"}`)
	view := decodePracticeData[environmentOperationView](t, recorder)
	deadline := time.Now().Add(5 * time.Second)
	for {
		statusReq := withPracticeIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/practice-environments/requests/"+view.ID, nil), "kevin", "personal-kevin")
		statusReq.SetPathValue("id", view.ID)
		statusRec := httptest.NewRecorder()
		handler.Status(statusRec, statusReq)
		current := decodePracticeData[environmentOperationView](t, statusRec)
		if current.Status == "failed" {
			if current.Error == "" {
				t.Fatalf("failed operation carries no error: %#v", current)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation never failed: %#v", current)
		}
		time.Sleep(10 * time.Millisecond)
	}
	otherReq := withPracticeIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/practice-environments/requests/"+view.ID, nil), "mallory", "personal-mallory")
	otherReq.SetPathValue("id", view.ID)
	otherRec := httptest.NewRecorder()
	handler.Status(otherRec, otherReq)
	if otherRec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want cross-owner operations hidden", otherRec.Code)
	}
}

func TestEnvironmentRequestValidation(t *testing.T) {
	handler := NewEnvironmentsHandler(&envintegration.OperationStore{Pipeline: instantPipeline("session-1", nil)})
	recorder := requestEnvironment(t, handler, "kevin", "personal-kevin", `{"objective":"no technology"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want technology required", recorder.Code)
	}
	broken := NewEnvironmentsHandler(nil)
	brokenRec := requestEnvironment(t, broken, "kevin", "personal-kevin", `{"technology":"demo"}`)
	if brokenRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want explicit unconfigured", brokenRec.Code)
	}
}
